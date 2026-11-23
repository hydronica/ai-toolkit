package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/hydronica/toml"
)

// ReadFile decodes the TOML config at path into a generic document. A missing
// file yields an empty document so the first save can create it.
func ReadFile(path string) (map[string]interface{}, error) {
	doc := map[string]interface{}{}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return doc, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc, nil
}

// WriteFile encodes doc as TOML and replaces path. It writes a temp file in the
// same directory and renames it, so a failed save leaves the old file intact.
// The file mode is 0600 because the file can hold passwords.
func WriteFile(path string, doc map[string]interface{}) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(doc); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".db-query-*.toml")
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	// After a successful rename this path no longer exists, so the removal error is ignored.
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

// Entries returns the [[databases]] tables in a config document.
func Entries(doc map[string]interface{}) ([]map[string]interface{}, error) {
	raw, ok := doc["databases"]
	if !ok {
		return nil, nil
	}
	switch v := raw.(type) {
	case []map[string]interface{}:
		return v, nil
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(v))
		for i, item := range v {
			table, ok := item.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("databases[%d] must be a table", i)
			}
			out = append(out, table)
		}
		return out, nil
	default:
		return nil, errors.New("databases must be an array of tables")
	}
}

// SetEntries replaces the [[databases]] tables in a config document. An empty
// list removes the key, because the loader rejects an empty inline array.
func SetEntries(doc map[string]interface{}, entries []map[string]interface{}) {
	if len(entries) == 0 {
		delete(doc, "databases")
		return
	}
	doc["databases"] = entries
}

// DecodeEntry turns one raw [[databases]] table into its typed config. It does
// not validate the result; call Validate before use.
func DecodeEntry(raw map[string]interface{}) (Database, error) {
	typ, _ := raw["type"].(string)
	return decodeEntry(normalizeType(typ), raw)
}

// FieldNames lists the TOML keys a database type accepts, in struct order.
func FieldNames(typ string) ([]string, error) {
	entry, err := DecodeEntry(map[string]interface{}{"type": typ})
	if err != nil {
		return nil, err
	}
	t := reflect.TypeOf(entry).Elem()
	names := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		tag := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if tag != "" && tag != "-" {
			names = append(names, tag)
		}
	}
	return names, nil
}

func normalizeType(typ string) string {
	return strings.ToLower(strings.TrimSpace(typ))
}
