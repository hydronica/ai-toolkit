package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/hydronica/go-config"
)

type Config struct {
	Databases []Database `toml:"databases"`
}

type Database struct {
	Name string `toml:"name"`
	Type string `toml:"type"`

	// Oracle
	Connection string `toml:"connection"`

	// Postgres (fields mirror go-rapi)
	Host                  string `toml:"host"`
	Username              string `toml:"username"`
	Password              string `toml:"password"`
	DB                    string `toml:"db"`
	SSLMode               string `toml:"sslmode"`
	SSLCert               string `toml:"sslcert"`
	SSLKey                string `toml:"sslkey"`
	SSLRootcert           string `toml:"sslrootcert"`
	SSLSkipHostnameVerify bool   `toml:"ssl_skip_hostname_verify"`

	// BigQuery (supports signal-api field names: bq-project, bq-dataset, bq-auth)
	Project     string `toml:"project"`
	Dataset     string `toml:"dataset"`
	Location    string `toml:"location"`
	Credentials string `toml:"credentials"`
	BQProject   string `toml:"bq-project"`
	BQDataset   string `toml:"bq-dataset"`
	BQAuth      string `toml:"bq-auth"`
	// Auth controls credential source: "adc" (gcloud/local default) or "service_account" (JSON key file).
	// When unset, uses service_account if credentials/bq-auth is set, otherwise adc.
	Auth string `toml:"auth"`

	// MongoDB (fields mirror go-rapi)
	URI    string `toml:"uri"`
	DBName string `toml:"db_name"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{}
	if err := config.LoadFile(path, cfg); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func DefaultPath() string {
	if path := strings.TrimSpace(os.Getenv("DB_QUERY_CONFIG")); path != "" {
		return path
	}
	return "config.toml"
}

func (c *Config) Validate() error {
	if len(c.Databases) == 0 {
		return errors.New("config must define at least one [[databases]] entry")
	}

	seen := make(map[string]struct{}, len(c.Databases))
	for i := range c.Databases {
		db := &c.Databases[i]
		name := strings.TrimSpace(db.Name)
		if name == "" {
			return fmt.Errorf("databases[%d]: name is required", i)
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("duplicate database name %q", name)
		}
		seen[name] = struct{}{}
		db.Name = name
		db.Type = strings.ToLower(strings.TrimSpace(db.Type))

		if err := db.validate(); err != nil {
			return fmt.Errorf("database %q: %w", name, err)
		}
	}
	return nil
}

func (d *Database) validate() error {
	switch d.Type {
	case "oracle":
		if strings.TrimSpace(d.Connection) == "" {
			return errors.New("oracle requires connection")
		}
	case "postgres":
		if d.Host == "" || d.DB == "" || d.Username == "" {
			return errors.New("postgres requires host, db, and username")
		}
		if d.SSLSkipHostnameVerify && strings.TrimSpace(d.SSLRootcert) == "" {
			return errors.New("postgres ssl_skip_hostname_verify requires sslrootcert")
		}
	case "mysql":
		if d.Host == "" || d.DB == "" || d.Username == "" {
			return errors.New("mysql requires host, db, and username")
		}
	case "sqlite":
		if strings.TrimSpace(d.Connection) == "" && strings.TrimSpace(d.DB) == "" {
			return errors.New("sqlite requires db (file path) or connection")
		}
	case "bigquery":
		d.normalizeBigQueryFields()
		if strings.TrimSpace(d.Project) == "" {
			return errors.New("bigquery requires project (or bq-project)")
		}
		if err := d.validateBigQueryAuth(); err != nil {
			return err
		}
	case "mongo", "mongodb":
		if strings.TrimSpace(d.URI) == "" || strings.TrimSpace(d.DBName) == "" {
			return errors.New("mongo requires uri and db_name")
		}
		if d.Type == "mongodb" {
			d.Type = "mongo"
		}
	default:
		return fmt.Errorf("unsupported type %q (use oracle, postgres, mysql, sqlite, bigquery, or mongo)", d.Type)
	}
	return nil
}

func (d *Database) normalizeBigQueryFields() {
	if strings.TrimSpace(d.Project) == "" {
		d.Project = strings.TrimSpace(d.BQProject)
	}
	if strings.TrimSpace(d.Dataset) == "" {
		d.Dataset = strings.TrimSpace(d.BQDataset)
	}
	if strings.TrimSpace(d.Credentials) == "" {
		d.Credentials = strings.TrimSpace(d.BQAuth)
	}
	d.Auth = normalizeBigQueryAuthMode(d.Auth, d.Credentials)
}

func normalizeBigQueryAuthMode(auth, credentials string) string {
	auth = strings.ToLower(strings.TrimSpace(auth))
	switch auth {
	case "", "adc", "gcloud", "application_default", "application-default":
		if auth == "" && credentials != "" {
			return "service_account"
		}
		return "adc"
	case "service_account", "service-account", "credentials", "credentials_file", "credentials-file":
		return "service_account"
	default:
		return auth
	}
}

func (d *Database) validateBigQueryAuth() error {
	switch d.Auth {
	case "adc":
		return nil
	case "service_account":
		if strings.TrimSpace(d.Credentials) == "" {
			return errors.New(`bigquery auth "service_account" requires credentials (or bq-auth)`)
		}
		return nil
	default:
		return fmt.Errorf(`unsupported bigquery auth %q (use "adc" or "service_account")`, d.Auth)
	}
}

func (d *Database) UsesBigQueryADC() bool {
	return d.Auth == "adc"
}

func (c *Config) Find(name string) (*Database, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		if len(c.Databases) == 1 {
			return &c.Databases[0], nil
		}
		return nil, errors.New("database name required: pass -db (multiple databases configured)")
	}

	for i := range c.Databases {
		if c.Databases[i].Name == name {
			return &c.Databases[i], nil
		}
	}
	return nil, fmt.Errorf("database %q not found in config", name)
}

func (c *Config) Names() []string {
	names := make([]string, len(c.Databases))
	for i, db := range c.Databases {
		names[i] = db.Name
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
		out[i] = DatabaseSummary{Name: db.Name, Type: db.Type}
	}
	return out
}
