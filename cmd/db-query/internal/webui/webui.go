// Package webui serves a local page for adding, editing, and testing database
// entries. It listens on 127.0.0.1 only and requires a token printed at startup.
package webui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	_ "embed"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"
	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/db"
)

//go:embed index.html
var indexHTML string

const (
	tokenHeader      = "X-DB-Query-Token"
	tokenPlaceholder = "__TOKEN__"
	testTimeout      = 15 * time.Second
	maxBodyBytes     = 1 << 20
)

// databaseTypes lists the types the page offers, in dropdown order.
var databaseTypes = []string{"oracle", "postgres", "mysql", "sqlite", "bigquery", "mongo"}

// secretFields are never returned to the browser. The page only learns whether
// a value is saved, and a blank field on save keeps the saved value.
var secretFields = map[string]bool{"password": true, "connection": true, "uri": true}

// boolFields render as checkboxes.
var boolFields = map[string]bool{"ssl_skip_hostname_verify": true}

type server struct {
	configPath string
	token      string
	port       int
	mu         sync.Mutex // serializes read-modify-write of the config file
}

// Serve listens on a random 127.0.0.1 port and serves the settings page until
// ctx is canceled. The printed URL carries the token, and every API call must
// send it in a header.
func Serve(ctx context.Context, configPath string, out io.Writer) error {
	token, err := newToken()
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		return errors.New("listen: unexpected address type")
	}

	s := &server{configPath: configPath, token: token, port: addr.Port}
	srv := &http.Server{Handler: s.routes(), ReadHeaderTimeout: 5 * time.Second}

	fmt.Fprintf(out, "db-query settings: http://127.0.0.1:%d/?token=%s\n", s.port, s.token)
	fmt.Fprintf(out, "config file: %s\n", configPath)
	fmt.Fprintln(out, "Press Ctrl-C to stop.")

	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /api/types", s.requireToken(s.handleTypes))
	mux.HandleFunc("GET /api/databases", s.requireToken(s.handleList))
	mux.HandleFunc("POST /api/databases", s.requireToken(s.handleSave))
	mux.HandleFunc("DELETE /api/databases/{name}", s.requireToken(s.handleDelete))
	mux.HandleFunc("POST /api/test", s.requireToken(s.handleTest))
	return s.requireLocalHost(mux)
}

// requireLocalHost rejects any Host other than this server's loopback address.
// Without the check, a hostile site could resolve its own name to 127.0.0.1
// (DNS rebinding) and reach the page.
func (s *server) requireLocalHost(next http.Handler) http.Handler {
	allowed := map[string]bool{
		fmt.Sprintf("127.0.0.1:%d", s.port): true,
		fmt.Sprintf("localhost:%d", s.port): true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowed[r.Host] {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) requireToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.tokenMatches(r.Header.Get(tokenHeader)) {
			writeError(w, http.StatusUnauthorized, errors.New("missing or invalid token"))
			return
		}
		next(w, r)
	}
}

func (s *server) tokenMatches(got string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if !s.tokenMatches(r.URL.Query().Get("token")) {
		http.Error(w, "open the settings URL printed by db-query -ui", http.StatusUnauthorized)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; form-action 'none'")
	_, _ = io.WriteString(w, strings.ReplaceAll(indexHTML, tokenPlaceholder, s.token))
}

type typeInfo struct {
	Type   string   `json:"type"`
	Fields []string `json:"fields"`
	Secret []string `json:"secret"`
	Bool   []string `json:"bool"`
}

func (s *server) handleTypes(w http.ResponseWriter, _ *http.Request) {
	infos := make([]typeInfo, 0, len(databaseTypes))
	for _, typ := range databaseTypes {
		fields, err := config.FieldNames(typ)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		info := typeInfo{Type: typ, Fields: fields, Secret: []string{}, Bool: []string{}}
		for _, f := range fields {
			if secretFields[f] {
				info.Secret = append(info.Secret, f)
			}
			if boolFields[f] {
				info.Bool = append(info.Bool, f)
			}
		}
		infos = append(infos, info)
	}
	writeJSON(w, http.StatusOK, infos)
}

type entryView struct {
	Name   string                 `json:"name"`
	Type   string                 `json:"type"`
	Values map[string]interface{} `json:"values"`
	Saved  []string               `json:"saved_secrets"`
}

func (s *server) handleList(w http.ResponseWriter, _ *http.Request) {
	// Reads need no lock: saves replace the file with a rename, so a read sees the old or new file.
	entries, err := s.loadEntries()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	views := make([]entryView, 0, len(entries))
	for _, e := range entries {
		views = append(views, viewOf(e))
	}
	writeJSON(w, http.StatusOK, views)
}

// viewOf omits secret values and reports only which of them are saved.
func viewOf(entry map[string]interface{}) entryView {
	name, _ := entry["name"].(string)
	typ, _ := entry["type"].(string)
	view := entryView{Name: name, Type: typ, Values: map[string]interface{}{}, Saved: []string{}}
	for key, val := range entry {
		if key == "name" || key == "type" {
			continue
		}
		if secretFields[key] {
			if str, ok := val.(string); !ok || str != "" {
				view.Saved = append(view.Saved, key)
			}
			continue
		}
		view.Values[key] = val
	}
	sort.Strings(view.Saved)
	return view
}

type saveRequest struct {
	OriginalName string                 `json:"original_name"`
	Type         string                 `json:"type"`
	Values       map[string]interface{} `json:"values"`
}

type saveResult struct {
	OK   bool   `json:"ok"`
	Name string `json:"name"`
}

func (s *server) handleSave(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeRequest(w, r)
	if !ok {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	doc, entries, err := s.loadDocument()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	idx := -1
	if req.OriginalName != "" {
		idx = findEntry(entries, req.OriginalName)
		if idx < 0 {
			writeError(w, http.StatusNotFound, fmt.Errorf("database %q not found", req.OriginalName))
			return
		}
	}

	var existing map[string]interface{}
	if idx >= 0 {
		existing = entries[idx]
	}
	entry, err := buildEntry(req.Type, req.Values, existing)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := checkEntry(entry); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	name, _ := entry["name"].(string)
	if other := findEntry(entries, name); other >= 0 && other != idx {
		writeError(w, http.StatusConflict, fmt.Errorf("database %q already exists", name))
		return
	}

	if idx >= 0 {
		entries[idx] = entry
	} else {
		entries = append(entries, entry)
	}
	if err := s.writeEntries(doc, entries); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, saveResult{OK: true, Name: name})
}

func (s *server) handleDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	s.mu.Lock()
	defer s.mu.Unlock()

	doc, entries, err := s.loadDocument()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	idx := findEntry(entries, name)
	if idx < 0 {
		writeError(w, http.StatusNotFound, fmt.Errorf("database %q not found", name))
		return
	}
	entries = slices.Delete(entries, idx, idx+1)
	if err := s.writeEntries(doc, entries); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, saveResult{OK: true, Name: name})
}

type testResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// handleTest connects with the submitted settings and pings. It never writes
// the config file, so a failed test cannot change saved entries.
func (s *server) handleTest(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeRequest(w, r)
	if !ok {
		return
	}

	s.mu.Lock()
	entries, err := s.loadEntries()
	s.mu.Unlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var existing map[string]interface{}
	if idx := findEntry(entries, req.OriginalName); req.OriginalName != "" && idx >= 0 {
		existing = entries[idx]
	}

	entry, err := buildEntry(req.Type, req.Values, existing)
	if err != nil {
		writeJSON(w, http.StatusOK, testResult{Error: err.Error()})
		return
	}
	secrets := secretValues(entry)
	dbCfg, err := checkEntry(entry)
	if err != nil {
		writeJSON(w, http.StatusOK, testResult{Error: redact(err.Error(), secrets)})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), testTimeout)
	defer cancel()
	runner, err := db.Connect(ctx, dbCfg)
	if err != nil {
		writeJSON(w, http.StatusOK, testResult{Error: redact(err.Error(), secrets)})
		return
	}
	// The close error adds nothing after a connection test, so it is discarded.
	defer func() { _ = runner.Close(ctx) }()

	if err := runner.Ping(ctx, db.QueryOptions{}); err != nil {
		writeJSON(w, http.StatusOK, testResult{Error: redact(err.Error(), secrets)})
		return
	}
	writeJSON(w, http.StatusOK, testResult{OK: true, Message: fmt.Sprintf("connected to %s (%s)", dbCfg.Name(), dbCfg.Type())})
}

// buildEntry merges submitted values into a raw config entry. A blank secret
// keeps the value from existing, so the browser never has to send a saved
// password back. Unknown fields are rejected so the file holds only keys the
// loader understands.
func buildEntry(typ string, values map[string]interface{}, existing map[string]interface{}) (map[string]interface{}, error) {
	typ = strings.ToLower(strings.TrimSpace(typ))
	fields, err := config.FieldNames(typ)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(fields))
	for _, f := range fields {
		allowed[f] = true
	}

	entry := map[string]interface{}{"type": typ}
	for key, raw := range values {
		if !allowed[key] {
			return nil, fmt.Errorf("field %q is not valid for %s", key, typ)
		}
		val := raw
		if str, ok := raw.(string); ok {
			val = strings.TrimSpace(str)
		}
		if secretFields[key] && val == "" {
			if old, ok := existing[key]; ok {
				entry[key] = old
			}
			continue
		}
		if val == "" || val == nil {
			continue
		}
		entry[key] = val
	}

	if name, _ := entry["name"].(string); name == "" {
		return nil, errors.New("name is required")
	}
	return entry, nil
}

// checkEntry decodes and validates an entry with the same rules the CLI uses.
func checkEntry(entry map[string]interface{}) (config.Database, error) {
	dbCfg, err := config.DecodeEntry(entry)
	if err != nil {
		return nil, err
	}
	if err := dbCfg.Validate(); err != nil {
		return nil, err
	}
	return dbCfg, nil
}

func secretValues(entry map[string]interface{}) []string {
	var out []string
	for key, val := range entry {
		if str, ok := val.(string); ok && secretFields[key] && str != "" {
			out = append(out, str)
		}
	}
	return out
}

// redact removes secret values from a message so driver errors cannot echo them.
func redact(msg string, secrets []string) string {
	for _, secret := range secrets {
		// An empty secret would match between every character, so skip it.
		if secret == "" {
			continue
		}
		msg = strings.ReplaceAll(msg, secret, "********")
	}
	return msg
}

func (s *server) loadEntries() ([]map[string]interface{}, error) {
	_, entries, err := s.loadDocument()
	return entries, err
}

func (s *server) loadDocument() (map[string]interface{}, []map[string]interface{}, error) {
	doc, err := config.ReadFile(s.configPath)
	if err != nil {
		return nil, nil, err
	}
	entries, err := config.Entries(doc)
	if err != nil {
		return nil, nil, err
	}
	return doc, entries, nil
}

func (s *server) writeEntries(doc map[string]interface{}, entries []map[string]interface{}) error {
	config.SetEntries(doc, entries)
	return config.WriteFile(s.configPath, doc)
}

func findEntry(entries []map[string]interface{}, name string) int {
	for i, entry := range entries {
		if n, _ := entry["name"].(string); n == name {
			return i
		}
	}
	return -1
}

func decodeRequest(w http.ResponseWriter, r *http.Request) (saveRequest, bool) {
	var req saveRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return req, false
	}
	return req, true
}

type apiError struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, apiError{Error: err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// An encode error here means the client went away; there is no one left to tell.
	_ = json.NewEncoder(w).Encode(v)
}
