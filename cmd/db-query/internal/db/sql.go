package db

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"

	_ "github.com/lib/pq"
	_ "github.com/sijms/go-ora/v2"
	_ "modernc.org/sqlite"
)

type sqlRunner struct {
	db *sqlx.DB
}

func connectSQL(ctx context.Context, dbCfg *config.Database) (*sqlRunner, error) {
	driver, dsn, err := sqlConnectionDetails(dbCfg)
	if err != nil {
		return nil, err
	}

	db, err := sqlx.Connect(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	if dbCfg.Type == "oracle" {
		db.Mapper = reflectx.NewMapperFunc("oracle", func(str string) string { return str })
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return &sqlRunner{db: db}, nil
}

func (r *sqlRunner) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

func (r *sqlRunner) Close() error {
	return r.db.Close()
}

func (r *sqlRunner) RunQuery(ctx context.Context, query string, limit int) (QueryOutput, error) {
	rows, err := r.db.QueryxContext(ctx, query)
	if err != nil {
		return QueryOutput{}, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return QueryOutput{}, fmt.Errorf("columns: %w", err)
	}

	output := QueryOutput{
		Columns: columns,
		Rows:    make([]map[string]interface{}, 0),
	}

	for rows.Next() {
		if limit > 0 && output.RowCount >= limit {
			output.Truncated = true
			break
		}

		row := make(map[string]interface{})
		if err := rows.MapScan(row); err != nil {
			return QueryOutput{}, fmt.Errorf("scan row: %w", err)
		}

		for key, value := range row {
			row[key] = normalizeSQLValue(value)
		}

		output.Rows = append(output.Rows, row)
		output.RowCount++
	}
	if err := rows.Err(); err != nil {
		return QueryOutput{}, fmt.Errorf("rows: %w", err)
	}

	return output, nil
}

func sqlConnectionDetails(dbCfg *config.Database) (driver, dsn string, err error) {
	switch dbCfg.Type {
	case "oracle":
		return "oracle", dbCfg.Connection, nil
	case "postgres":
		return "postgres", postgresDSN(dbCfg), nil
	case "mysql":
		return "mysql", mysqlDSN(dbCfg), nil
	case "sqlite":
		if conn := strings.TrimSpace(dbCfg.Connection); conn != "" {
			return "sqlite", conn, nil
		}
		return "sqlite", sqliteDSN(dbCfg), nil
	default:
		return "", "", fmt.Errorf("unsupported database type %q", dbCfg.Type)
	}
}

func postgresDSN(dbCfg *config.Database) string {
	if dbCfg.SSLMode == "" || dbCfg.SSLMode == "disable" {
		return fmt.Sprintf(
			"postgres://%s:%s@%s/%s?connect_timeout=5&sslmode=disable",
			url.QueryEscape(dbCfg.Username),
			url.QueryEscape(dbCfg.Password),
			dbCfg.Host,
			dbCfg.DB,
		)
	}

	return fmt.Sprintf(
		"postgres://%s:%s@%s/%s?connect_timeout=5&sslmode=%s&sslcert=%s&sslkey=%s&sslrootcert=%s",
		url.QueryEscape(dbCfg.Username),
		url.QueryEscape(dbCfg.Password),
		dbCfg.Host,
		dbCfg.DB,
		url.QueryEscape(dbCfg.SSLMode),
		url.QueryEscape(dbCfg.SSLCert),
		url.QueryEscape(dbCfg.SSLKey),
		url.QueryEscape(dbCfg.SSLRootcert),
	)
}

func mysqlDSN(dbCfg *config.Database) string {
	addr := dbCfg.Host
	if !strings.Contains(addr, ":") {
		addr += ":3306"
	}

	cfg := mysql.Config{
		User:                 dbCfg.Username,
		Passwd:               dbCfg.Password,
		Net:                  "tcp",
		Addr:                 addr,
		DBName:               dbCfg.DB,
		ParseTime:            true,
		Timeout:              5 * time.Second,
		AllowNativePasswords: true,
	}
	return cfg.FormatDSN()
}

func sqliteDSN(dbCfg *config.Database) string {
	path := strings.TrimSpace(dbCfg.DB)
	if path == ":memory:" {
		return "file::memory:?mode=ro"
	}
	if strings.HasPrefix(path, "file:") {
		return ensureSQLiteReadOnly(path)
	}
	return ensureSQLiteReadOnly("file:" + path)
}

func ensureSQLiteReadOnly(dsn string) string {
	if strings.Contains(dsn, "mode=") {
		return dsn
	}
	if strings.Contains(dsn, "?") {
		return dsn + "&mode=ro"
	}
	return dsn + "?mode=ro"
}

func sortedKeys(row map[string]interface{}) []string {
	keys := make([]string, 0, len(row))
	for key := range row {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func normalizeSQLValue(value interface{}) interface{} {
	switch v := value.(type) {
	case nil:
		return nil
	case []byte:
		return string(v)
	case time.Time:
		return v.UTC().Format(time.RFC3339Nano)
	default:
		return v
	}
}
