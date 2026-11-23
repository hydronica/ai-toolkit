package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"
	"github.com/hydronica/trial"
)

const (
	testToken = "test-token"
	testPort  = 4321
	localHost = "127.0.0.1:4321"
)

const oneDB = `
[[databases]]
name = "app"
type = "sqlite"
db = "/tmp/app.db"
`

const twoDBs = oneDB + `
[[databases]]
name = "local"
type = "sqlite"
db = "/tmp/local.db"
`

func testServer(path string) *server {
	return &server{configPath: path, token: testToken, port: testPort}
}

func request(s *server, method, target, host, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Host = host
	if token != "" {
		req.Header.Set(tokenHeader, token)
	}
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	return rec
}

type buildInput struct {
	typ      string
	values   map[string]interface{}
	existing map[string]interface{}
}

func TestBuildEntry(t *testing.T) {
	fn := func(in buildInput) (map[string]interface{}, error) {
		return buildEntry(in.typ, in.values, in.existing)
	}
	cases := trial.Cases[buildInput, map[string]interface{}]{
		"keeps saved password when the field is blank": {
			Input: buildInput{
				typ:      "postgres",
				values:   map[string]interface{}{"name": "pg", "host": "h", "db": "d", "username": "u", "password": ""},
				existing: map[string]interface{}{"name": "pg", "type": "postgres", "password": "old"},
			},
			Expected: map[string]interface{}{"type": "postgres", "name": "pg", "host": "h", "db": "d", "username": "u", "password": "old"},
		},
		"omits a blank password for a new entry": {
			Input: buildInput{
				typ:    "postgres",
				values: map[string]interface{}{"name": "pg", "host": "h", "db": "d", "username": "u", "password": ""},
			},
			Expected: map[string]interface{}{"type": "postgres", "name": "pg", "host": "h", "db": "d", "username": "u"},
		},
		"keeps an unchecked checkbox as false": {
			Input: buildInput{
				typ:    "postgres",
				values: map[string]interface{}{"name": "pg", "ssl_skip_hostname_verify": false},
			},
			Expected: map[string]interface{}{"type": "postgres", "name": "pg", "ssl_skip_hostname_verify": false},
		},
		"trims text values": {
			Input: buildInput{
				typ:    "mysql",
				values: map[string]interface{}{"name": "  app  ", "host": " h "},
			},
			Expected: map[string]interface{}{"type": "mysql", "name": "app", "host": "h"},
		},
		"rejects a field the type does not accept": {
			Input: buildInput{
				typ:    "postgres",
				values: map[string]interface{}{"name": "pg", "uri": "mongodb://x"},
			},
			ExpectedErr: fmt.Errorf(`field "uri" is not valid for postgres`),
		},
		"requires a name": {
			Input: buildInput{
				typ:    "postgres",
				values: map[string]interface{}{"host": "h"},
			},
			ExpectedErr: fmt.Errorf("name is required"),
		},
		"rejects an unknown type": {
			Input: buildInput{
				typ:    "redis",
				values: map[string]interface{}{"name": "cache"},
			},
			ExpectedErr: fmt.Errorf("unsupported type"),
		},
	}
	trial.New(fn, cases).SubTest(t)
}

type guardInput struct {
	method, target, host, token string
}

func TestServer_Guard(t *testing.T) {
	fn := func(in guardInput) (int, error) {
		s := testServer(filepath.Join(t.TempDir(), "config.toml"))
		return request(s, in.method, in.target, in.host, in.token, "").Code, nil
	}
	cases := trial.Cases[guardInput, int]{
		"page without token is unauthorized": {
			Input:    guardInput{method: "GET", target: "/", host: localHost},
			Expected: http.StatusUnauthorized,
		},
		"page with token is served": {
			Input:    guardInput{method: "GET", target: "/?token=" + testToken, host: localHost},
			Expected: http.StatusOK,
		},
		"page from a foreign host is forbidden": {
			Input:    guardInput{method: "GET", target: "/?token=" + testToken, host: "evil.example:4321"},
			Expected: http.StatusForbidden,
		},
		"api without token header is unauthorized": {
			Input:    guardInput{method: "GET", target: "/api/databases", host: localHost},
			Expected: http.StatusUnauthorized,
		},
		"api with wrong token is unauthorized": {
			Input:    guardInput{method: "GET", target: "/api/databases", host: localHost, token: "nope"},
			Expected: http.StatusUnauthorized,
		},
		"api with token lists databases": {
			Input:    guardInput{method: "GET", target: "/api/databases", host: localHost, token: testToken},
			Expected: http.StatusOK,
		},
	}
	trial.New(fn, cases).SubTest(t)
}

type saveCase struct {
	config string
	method string
	target string
	body   string
}

// TestServer_Save runs requests through the real routes against a temp config
// file. The output is the database names left in the file, in order.
func TestServer_Save(t *testing.T) {
	fn := func(in saveCase) (string, error) {
		path := filepath.Join(t.TempDir(), "config.toml")
		if in.config != "" {
			if err := os.WriteFile(path, []byte(in.config), 0o600); err != nil {
				return "", err
			}
		}
		rec := request(testServer(path), in.method, in.target, localHost, testToken, in.body)
		if rec.Code >= http.StatusBadRequest {
			return "", fmt.Errorf("status %d: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
		}
		doc, err := config.ReadFile(path)
		if err != nil {
			return "", err
		}
		entries, err := config.Entries(doc)
		if err != nil {
			return "", err
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			name, _ := e["name"].(string)
			names = append(names, name)
		}
		return strings.Join(names, ","), nil
	}

	cases := trial.Cases[saveCase, string]{
		"adds the first database": {
			Input: saveCase{
				method: "POST",
				target: "/api/databases",
				body:   `{"type":"sqlite","values":{"name":"local","db":"/tmp/local.db"}}`,
			},
			Expected: "local",
		},
		"adds after existing entries": {
			Input: saveCase{
				config: oneDB,
				method: "POST",
				target: "/api/databases",
				body:   `{"type":"sqlite","values":{"name":"local","db":"/tmp/local.db"}}`,
			},
			Expected: "app,local",
		},
		"rejects a duplicate name": {
			Input: saveCase{
				config: oneDB,
				method: "POST",
				target: "/api/databases",
				body:   `{"type":"sqlite","values":{"name":"app","db":"/tmp/other.db"}}`,
			},
			ExpectedErr: fmt.Errorf(`already exists`),
		},
		"updates an entry in place": {
			Input: saveCase{
				config: twoDBs,
				method: "POST",
				target: "/api/databases",
				body:   `{"original_name":"app","type":"sqlite","values":{"name":"app","db":"/tmp/new.db"}}`,
			},
			Expected: "app,local",
		},
		"renames an entry": {
			Input: saveCase{
				config: oneDB,
				method: "POST",
				target: "/api/databases",
				body:   `{"original_name":"app","type":"sqlite","values":{"name":"reporting","db":"/tmp/app.db"}}`,
			},
			Expected: "reporting",
		},
		"rejects an unknown field before writing": {
			Input: saveCase{
				config: oneDB,
				method: "POST",
				target: "/api/databases",
				body:   `{"type":"sqlite","values":{"name":"local","uri":"x"}}`,
			},
			ExpectedErr: fmt.Errorf(`is not valid for sqlite`),
		},
		"delete removes the entry": {
			Input: saveCase{
				config: twoDBs,
				method: "DELETE",
				target: "/api/databases/app",
			},
			Expected: "local",
		},
		"delete reports a missing entry": {
			Input: saveCase{
				config: oneDB,
				method: "DELETE",
				target: "/api/databases/missing",
			},
			ExpectedErr: fmt.Errorf("not found"),
		},
	}
	trial.New(fn, cases).SubTest(t)
}

// TestServer_TestDoesNotSave checks that a failed connection test leaves the
// config file byte-for-byte unchanged.
func TestServer_TestDoesNotSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(oneDB), 0o600); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"sqlite","values":{"name":"broken","db":"/nonexistent/dir/broken.db"}}`
	rec := request(testServer(path), "POST", "/api/test", localHost, testToken, body)

	var res testResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode response: %v (body %q)", err, rec.Body.String())
	}
	if res.OK || res.Error == "" {
		t.Fatalf("test of unreachable database got ok=%v error=%q; want a failure with a message", res.OK, res.Error)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != oneDB {
		t.Fatalf("Test changed the config file:\n%s", after)
	}
}

type redactInput struct {
	msg     string
	secrets []string
}

func TestRedact(t *testing.T) {
	fn := func(in redactInput) (string, error) {
		return redact(in.msg, in.secrets), nil
	}
	cases := trial.Cases[redactInput, string]{
		"replaces every secret occurrence": {
			Input: redactInput{
				msg:     `dial postgres://u:hunter2@h/db: bad password "hunter2"`,
				secrets: []string{"hunter2"},
			},
			Expected: `dial postgres://u:********@h/db: bad password "********"`,
		},
		"ignores empty secrets": {
			Input:    redactInput{msg: "timeout", secrets: []string{""}},
			Expected: "timeout",
		},
	}
	trial.New(fn, cases).SubTest(t)
}
