package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Query           string        `flag:"query" comment:"SQL SELECT query to execute"`
	DB              string        `flag:"db" comment:"Database name from config"`
	ListDBs         bool          `flag:"list-dbs" comment:"List configured databases and exit"`
	Dataset         string        `flag:"dataset" comment:"BigQuery: default dataset for unqualified table names"`
	Format          string        `flag:"format" comment:"Output format: json, csv, table"`
	Limit           int           `flag:"limit" comment:"Maximum rows to return (0 = unlimited); for -list-schema, maximum objects"`
	Timeout         time.Duration `flag:"timeout" comment:"Query timeout"`
	Ping            bool          `flag:"ping" comment:"Test database connection and exit"`
	ListSchema      bool          `flag:"list-schema" comment:"List tables, views, or collections and their columns, then exit"`
	ListCollections bool          `flag:"list-collections" comment:"Deprecated alias of -list-schema"`
	MCP             bool          `flag:"mcp" comment:"Start MCP stdio server"`

	Databases DatabaseList `toml:"databases" flag:"-"`
}

func DefaultPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ".conf.toml"
	}
	path := filepath.Join(filepath.Dir(exe), ".conf.toml")
	return path
}

func (c *Config) Validate() error {
	if len(c.Databases) == 0 {
		return errors.New("config must define at least one [[databases]] entry")
	}

	seen := make(map[string]struct{}, len(c.Databases))
	for i, db := range c.Databases {
		name := strings.TrimSpace(db.Name())
		if name == "" {
			return fmt.Errorf("databases[%d]: name is required", i)
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("duplicate database name %q", name)
		}
		seen[name] = struct{}{}

		if err := db.Validate(); err != nil {
			return fmt.Errorf("database %q: %w", name, err)
		}
	}
	return nil
}

func (c *Config) Find(name string) (Database, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		if len(c.Databases) == 1 {
			return c.Databases[0], nil
		}
		return nil, errors.New("database name required: pass -db (multiple databases configured)")
	}

	for _, db := range c.Databases {
		if db.Name() == name {
			return db, nil
		}
	}
	return nil, fmt.Errorf("database %q not found in config", name)
}

func (c *Config) Names() []string {
	names := make([]string, len(c.Databases))
	for i, db := range c.Databases {
		names[i] = db.Name()
	}
	return names
}

type Summary struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func (c *Config) Summaries() []Summary {
	out := make([]Summary, len(c.Databases))
	for i, db := range c.Databases {
		out[i] = Summary{Name: db.Name(), Type: db.Type()}
	}
	return out
}
