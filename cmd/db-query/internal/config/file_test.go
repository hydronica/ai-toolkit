package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hydronica/trial"
)

func TestFieldNames(t *testing.T) {
	cases := trial.Cases[string, []string]{
		"sqlite accepts name, connection, and db": {
			Input:    "sqlite",
			Expected: []string{"name", "connection", "db"},
		},
		"mongodb alias uses mongo fields": {
			Input:    "MongoDB",
			Expected: []string{"name", "uri", "db_name"},
		},
		"rejects unknown type": {
			Input:       "redis",
			ExpectedErr: errors.New("unsupported type"),
		},
	}
	trial.New(FieldNames, cases).SubTest(t)
}

func TestWriteFile(t *testing.T) {
	// fn writes the input TOML (when non-empty), appends one sqlite entry, and
	// returns the entry names read back from disk. An empty input means the file
	// does not exist yet.
	fn := func(in string) ([]string, error) {
		path := filepath.Join(t.TempDir(), "config.toml")
		if in != "" {
			if err := os.WriteFile(path, []byte(in), 0o600); err != nil {
				return nil, err
			}
		}
		doc, err := ReadFile(path)
		if err != nil {
			return nil, err
		}
		entries, err := Entries(doc)
		if err != nil {
			return nil, err
		}
		entries = append(entries, map[string]interface{}{
			"name": "added",
			"type": "sqlite",
			"db":   "/tmp/added.db",
		})
		SetEntries(doc, entries)
		if err := WriteFile(path, doc); err != nil {
			return nil, err
		}

		doc, err = ReadFile(path)
		if err != nil {
			return nil, err
		}
		entries, err = Entries(doc)
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			name, _ := e["name"].(string)
			names = append(names, name)
		}
		return names, nil
	}

	cases := trial.Cases[string, []string]{
		"creates the file when it does not exist": {
			Input:    "",
			Expected: []string{"added"},
		},
		"keeps existing entries and appends new ones": {
			Input: `
[[databases]]
name = "reporting"
type = "mongo"
uri = "mongodb://localhost"
db_name = "reporting"

[[databases]]
name = "app"
type = "sqlite"
db = "/tmp/app.db"
`,
			Expected: []string{"reporting", "app", "added"},
		},
	}
	trial.New(fn, cases).SubTest(t)
}

func TestWriteFile_Mode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, map[string]interface{}{}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("WriteFile mode got %o want 600 (config can hold passwords)", got)
	}
}
