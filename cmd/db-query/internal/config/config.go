package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Databases DatabaseList `toml:"databases"`
}

func DefaultPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(filepath.Dir(exe), "config.toml")
}

func (c *Config) Validate() error {
	if len(c.Databases) == 0 {
		return errors.New("config must define at least one [[databases]] entry")
	}

	seen := make(map[string]struct{}, len(c.Databases))
	for i, db := range c.Databases {
		name := strings.TrimSpace(db.DatabaseName())
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

func (c *Config) Find(name string) (DatabaseConfig, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		if len(c.Databases) == 1 {
			return c.Databases[0], nil
		}
		return nil, errors.New("database name required: pass -db (multiple databases configured)")
	}

	for _, db := range c.Databases {
		if db.DatabaseName() == name {
			return db, nil
		}
	}
	return nil, fmt.Errorf("database %q not found in config", name)
}

func (c *Config) Names() []string {
	names := make([]string, len(c.Databases))
	for i, db := range c.Databases {
		names[i] = db.DatabaseName()
	}
	return names
}

type DatabaseSummary struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func (c *Config) Summaries() []DatabaseSummary {
	out := make([]DatabaseSummary, len(c.Databases))
	for i, db := range c.Databases {
		out[i] = DatabaseSummary{Name: db.DatabaseName(), Type: db.DatabaseType()}
	}
	return out
}
