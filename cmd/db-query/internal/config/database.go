package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// DatabaseConfig is implemented by all database connection configurations.
// The interface lives here because this package is the factory/orchestrator
// that dispatches based on the TOML "type" discriminator.
type DatabaseConfig interface {
	DatabaseName() string
	DatabaseType() string
	Validate() error
}

// DatabaseList is the TOML-decoded slice of database configs. It implements
// the hydronica/toml Unmarshaler interface to dispatch on the "type" field.
type DatabaseList []DatabaseConfig

func (dl *DatabaseList) UnmarshalTOML(data interface{}) error {
	entries, ok := data.([]map[string]interface{})
	if !ok {
		return errors.New("databases must be an array of tables")
	}

	for i, raw := range entries {
		typStr, _ := raw["type"].(string)
		typStr = strings.ToLower(strings.TrimSpace(typStr))

		cfg, err := decodeDatabaseEntry(typStr, raw)
		if err != nil {
			return fmt.Errorf("databases[%d]: %w", i, err)
		}
		*dl = append(*dl, cfg)
	}
	return nil
}

func decodeDatabaseEntry(typ string, raw map[string]interface{}) (DatabaseConfig, error) {
	switch typ {
	case "oracle":
		return decodeAs[OracleConfig](raw)
	case "postgres":
		return decodeAs[PostgresConfig](raw)
	case "mysql":
		return decodeAs[MySQLConfig](raw)
	case "sqlite":
		return decodeAs[SQLiteConfig](raw)
	case "bigquery":
		return decodeAs[BigQueryConfig](raw)
	case "mongo", "mongodb":
		return decodeAs[MongoConfig](raw)
	case "":
		return nil, errors.New("type is required")
	default:
		return nil, fmt.Errorf("unsupported type %q (use oracle, postgres, mysql, sqlite, bigquery, or mongo)", typ)
	}
}

// decodeAs uses a JSON roundtrip to decode the raw TOML map into a typed struct.
// TOML key names match JSON tags on the target structs.
func decodeAs[T any, PT interface {
	*T
	DatabaseConfig
}](raw map[string]interface{}) (PT, error) {
	cfg := PT(new(T))
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return cfg, nil
}

// OracleConfig connects via a connection URI string.
type OracleConfig struct {
	Name       string `json:"name" toml:"name"`
	Connection string `json:"connection" toml:"connection"`
}

func (c *OracleConfig) DatabaseName() string { return c.Name }
func (c *OracleConfig) DatabaseType() string { return "oracle" }

func (c *OracleConfig) Validate() error {
	if strings.TrimSpace(c.Connection) == "" {
		return errors.New("oracle requires connection")
	}
	return nil
}

// PostgresConfig supports either a Connection URI or individual fields.
type PostgresConfig struct {
	Name                  string `json:"name" toml:"name"`
	Connection            string `json:"connection" toml:"connection"`
	Host                  string `json:"host" toml:"host"`
	Username              string `json:"username" toml:"username"`
	Password              string `json:"password" toml:"password"`
	DB                    string `json:"db" toml:"db"`
	SSLMode               string `json:"sslmode" toml:"sslmode"`
	SSLCert               string `json:"sslcert" toml:"sslcert"`
	SSLKey                string `json:"sslkey" toml:"sslkey"`
	SSLRootcert           string `json:"sslrootcert" toml:"sslrootcert"`
	SSLSkipHostnameVerify bool   `json:"ssl_skip_hostname_verify" toml:"ssl_skip_hostname_verify"`
}

func (c *PostgresConfig) DatabaseName() string { return c.Name }
func (c *PostgresConfig) DatabaseType() string { return "postgres" }

func (c *PostgresConfig) Validate() error {
	if strings.TrimSpace(c.Connection) != "" {
		return nil
	}
	if c.Host == "" || c.DB == "" || c.Username == "" {
		return errors.New("postgres requires connection URI or host, db, and username")
	}
	if c.SSLSkipHostnameVerify && strings.TrimSpace(c.SSLRootcert) == "" {
		return errors.New("postgres ssl_skip_hostname_verify requires sslrootcert")
	}
	return nil
}

// DSN builds a connection string. If Connection is set it is returned directly.
func (c *PostgresConfig) DSN() string {
	if conn := strings.TrimSpace(c.Connection); conn != "" {
		return conn
	}
	if c.SSLMode == "" || c.SSLMode == "disable" {
		return fmt.Sprintf(
			"postgres://%s:%s@%s/%s?connect_timeout=5&sslmode=disable",
			url.QueryEscape(c.Username),
			url.QueryEscape(c.Password),
			c.Host,
			c.DB,
		)
	}
	return fmt.Sprintf(
		"postgres://%s:%s@%s/%s?connect_timeout=5&sslmode=%s&sslcert=%s&sslkey=%s&sslrootcert=%s",
		url.QueryEscape(c.Username),
		url.QueryEscape(c.Password),
		c.Host,
		c.DB,
		url.QueryEscape(c.SSLMode),
		url.QueryEscape(c.SSLCert),
		url.QueryEscape(c.SSLKey),
		url.QueryEscape(c.SSLRootcert),
	)
}

// MySQLConfig supports either a Connection DSN or individual fields.
type MySQLConfig struct {
	Name       string `json:"name" toml:"name"`
	Connection string `json:"connection" toml:"connection"`
	Host       string `json:"host" toml:"host"`
	Username   string `json:"username" toml:"username"`
	Password   string `json:"password" toml:"password"`
	DB         string `json:"db" toml:"db"`
}

func (c *MySQLConfig) DatabaseName() string { return c.Name }
func (c *MySQLConfig) DatabaseType() string { return "mysql" }

func (c *MySQLConfig) Validate() error {
	if strings.TrimSpace(c.Connection) != "" {
		return nil
	}
	if c.Host == "" || c.DB == "" || c.Username == "" {
		return errors.New("mysql requires connection DSN or host, db, and username")
	}
	return nil
}

// DSN builds a MySQL DSN. If Connection is set it is returned directly.
func (c *MySQLConfig) DSN() string {
	if conn := strings.TrimSpace(c.Connection); conn != "" {
		return conn
	}
	addr := c.Host
	if !strings.Contains(addr, ":") {
		addr += ":3306"
	}
	cfg := mysql.Config{
		User:                 c.Username,
		Passwd:               c.Password,
		Net:                  "tcp",
		Addr:                 addr,
		DBName:               c.DB,
		ParseTime:            true,
		Timeout:              5 * time.Second,
		AllowNativePasswords: true,
	}
	return cfg.FormatDSN()
}

// SQLiteConfig supports either a Connection URI or a file path via DB.
type SQLiteConfig struct {
	Name       string `json:"name" toml:"name"`
	Connection string `json:"connection" toml:"connection"`
	DB         string `json:"db" toml:"db"`
}

func (c *SQLiteConfig) DatabaseName() string { return c.Name }
func (c *SQLiteConfig) DatabaseType() string { return "sqlite" }

func (c *SQLiteConfig) Validate() error {
	if strings.TrimSpace(c.Connection) == "" && strings.TrimSpace(c.DB) == "" {
		return errors.New("sqlite requires db (file path) or connection")
	}
	return nil
}

// DSN returns the sqlite connection string with read-only mode enforced.
// In-memory databases are exempt from read-only since they start empty.
func (c *SQLiteConfig) DSN() string {
	if conn := strings.TrimSpace(c.Connection); conn != "" {
		if strings.Contains(conn, ":memory:") {
			return conn
		}
		return ensureSQLiteReadOnly(conn)
	}
	db := strings.TrimSpace(c.DB)
	switch {
	case db == ":memory:":
		return "file::memory:"
	case strings.HasPrefix(db, "file:"):
		return ensureSQLiteReadOnly(db)
	default:
		return ensureSQLiteReadOnly("file:" + db)
	}
}

func ensureSQLiteReadOnly(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	sep := strings.IndexByte(dsn, '?')
	if sep < 0 {
		return dsn + "?mode=ro"
	}
	return dsn[:sep] + "?" + setSQLiteModeReadOnly(dsn[sep+1:])
}

func setSQLiteModeReadOnly(query string) string {
	if query == "" {
		return "mode=ro"
	}
	parts := strings.Split(query, "&")
	modeSet := false
	for i, part := range parts {
		if part == "" {
			continue
		}
		key, _, _ := strings.Cut(part, "=")
		if strings.EqualFold(key, "mode") {
			parts[i] = "mode=ro"
			modeSet = true
		}
	}
	out := strings.Join(parts, "&")
	if !modeSet {
		return out + "&mode=ro"
	}
	return out
}

// BigQueryConfig stores credentials and project information for BigQuery.
type BigQueryConfig struct {
	Name        string `json:"name" toml:"name"`
	Project     string `json:"project" toml:"project"`
	Dataset     string `json:"dataset" toml:"dataset"`
	Location    string `json:"location" toml:"location"`
	Credentials string `json:"credentials" toml:"credentials"`
	BQProject   string `json:"bq-project" toml:"bq-project"`
	BQDataset   string `json:"bq-dataset" toml:"bq-dataset"`
	BQAuth      string `json:"bq-auth" toml:"bq-auth"`
	Auth        string `json:"auth" toml:"auth"`
}

func (c *BigQueryConfig) DatabaseName() string { return c.Name }
func (c *BigQueryConfig) DatabaseType() string { return "bigquery" }

func (c *BigQueryConfig) Validate() error {
	c.normalize()
	if strings.TrimSpace(c.Project) == "" {
		return errors.New("bigquery requires project (or bq-project)")
	}
	switch c.Auth {
	case "adc":
		return nil
	case "service_account":
		if strings.TrimSpace(c.Credentials) == "" {
			return errors.New(`bigquery auth "service_account" requires credentials (or bq-auth)`)
		}
		return nil
	default:
		return fmt.Errorf(`unsupported bigquery auth %q (use "adc" or "service_account")`, c.Auth)
	}
}

func (c *BigQueryConfig) normalize() {
	if strings.TrimSpace(c.Project) == "" {
		c.Project = strings.TrimSpace(c.BQProject)
	}
	if strings.TrimSpace(c.Dataset) == "" {
		c.Dataset = strings.TrimSpace(c.BQDataset)
	}
	if strings.TrimSpace(c.Credentials) == "" {
		c.Credentials = strings.TrimSpace(c.BQAuth)
	}
	c.Auth = normalizeBigQueryAuthMode(c.Auth, c.Credentials)
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

// UsesBigQueryADC reports whether the config uses Application Default Credentials.
func (c *BigQueryConfig) UsesBigQueryADC() bool {
	return c.Auth == "adc"
}

// MongoConfig stores URI and database name for MongoDB connections.
type MongoConfig struct {
	Name   string `json:"name" toml:"name"`
	Type   string `json:"type" toml:"type"`
	URI    string `json:"uri" toml:"uri"`
	DBName string `json:"db_name" toml:"db_name"`
}

func (c *MongoConfig) DatabaseName() string { return c.Name }
func (c *MongoConfig) DatabaseType() string { return "mongo" }

func (c *MongoConfig) Validate() error {
	if strings.TrimSpace(c.URI) == "" || strings.TrimSpace(c.DBName) == "" {
		return errors.New("mongo requires uri and db_name")
	}
	return nil
}
