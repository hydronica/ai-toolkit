package db

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/sijms/go-ora/v2"
	_ "modernc.org/sqlite"
)

type sqlRunner struct {
	*sqlx.DB
}

func connectOracle(ctx context.Context, cfg *config.Oracle) (*sqlRunner, error) {
	db, err := sqlx.Open("oracle", cfg.Connection)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	db.Mapper = reflectx.NewMapperFunc("oracle", func(str string) string { return str })

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect: %w", err)
	}
	return &sqlRunner{db}, nil
}

func connectMySQL(ctx context.Context, cfg *config.MySQL) (*sqlRunner, error) {
	db, err := sqlx.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect: %w", err)
	}
	return &sqlRunner{db}, nil
}

func connectSQLite(ctx context.Context, cfg *config.SQLite) (*sqlRunner, error) {
	db, err := sqlx.Open("sqlite", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect: %w", err)
	}
	return &sqlRunner{db}, nil
}

func (r *sqlRunner) Ping(ctx context.Context) error {
	return r.PingContext(ctx)
}

func (r *sqlRunner) RunQuery(ctx context.Context, query string, limit int) (QueryOutput, error) {
	rows, err := r.QueryxContext(ctx, query)
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
