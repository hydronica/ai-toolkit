package db

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"
)

var forbidden = regexp.MustCompile(`\b(INSERT|UPDATE|DELETE|DROP|ALTER|CREATE|TRUNCATE|MERGE|GRANT|REVOKE|EXEC|EXECUTE|CALL|COPY|REPLACE|RENAME|INTO)\b`)

var readOnlySideEffects = regexp.MustCompile(`\b(FOR\s+UPDATE|FOR\s+SHARE|LOCK\s+IN\s+SHARE\s+MODE)\b`)

func ValidateReadOnlyQuery(query string) error {
	trimmed := strings.TrimSpace(query)
	trimmed = strings.TrimSuffix(trimmed, ";")
	upper := strings.ToUpper(trimmed)

	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		return errors.New("only read-only SELECT queries are allowed")
	}
	if readOnlySideEffects.MatchString(upper) {
		return errors.New("query contains forbidden locking clauses; only read-only SELECT queries are allowed")
	}
	if forbidden.MatchString(upper) {
		return errors.New("query contains forbidden keywords; only read-only SELECT queries are allowed")
	}
	if strings.Count(trimmed, ";") > 0 {
		return errors.New("only a single SQL statement is allowed")
	}
	return nil
}

// QueryOptions controls per-query execution. Dataset applies to BigQuery only.
type QueryOptions struct {
	Limit   int
	Dataset string
}

type QueryOutput struct {
	Columns   []string                 `json:"columns"`
	Rows      []map[string]interface{} `json:"rows"`
	RowCount  int                      `json:"row_count"`
	Truncated bool                     `json:"truncated,omitempty"`
}

type Runner struct {
	sql      *sqlRunner
	bigquery *bigqueryRunner
	mongo    *mongoRunner
}

func Connect(ctx context.Context, cfg config.DatabaseConfig) (*Runner, error) {
	switch c := cfg.(type) {
	case *config.MongoConfig:
		r, err := connectMongo(ctx, c)
		if err != nil {
			return nil, err
		}
		return &Runner{mongo: r}, nil
	case *config.BigQueryConfig:
		r, err := connectBigQuery(ctx, c)
		if err != nil {
			return nil, err
		}
		return &Runner{bigquery: r}, nil
	case *config.PostgresConfig:
		r, err := connectPostgres(ctx, c)
		if err != nil {
			return nil, err
		}
		return &Runner{sql: r}, nil
	case *config.OracleConfig:
		r, err := connectOracle(ctx, c)
		if err != nil {
			return nil, err
		}
		return &Runner{sql: r}, nil
	case *config.MySQLConfig:
		r, err := connectMySQL(ctx, c)
		if err != nil {
			return nil, err
		}
		return &Runner{sql: r}, nil
	case *config.SQLiteConfig:
		r, err := connectSQLite(ctx, c)
		if err != nil {
			return nil, err
		}
		return &Runner{sql: r}, nil
	default:
		return nil, fmt.Errorf("unsupported database config type %T", cfg)
	}
}

func (r *Runner) Ping(ctx context.Context, opts QueryOptions) error {
	switch {
	case r.mongo != nil:
		return r.mongo.Ping(ctx)
	case r.bigquery != nil:
		return r.bigquery.Ping(ctx, opts)
	default:
		return r.sql.Ping(ctx)
	}
}

func (r *Runner) Close(ctx context.Context) error {
	switch {
	case r.mongo != nil:
		return r.mongo.Close(ctx)
	case r.bigquery != nil:
		return r.bigquery.Close()
	default:
		return r.sql.Close()
	}
}

func (r *Runner) RunQuery(ctx context.Context, query string, opts QueryOptions) (QueryOutput, error) {
	if r.mongo == nil {
		if err := ValidateReadOnlyQuery(query); err != nil {
			return QueryOutput{}, err
		}
	}
	switch {
	case r.mongo != nil:
		return r.mongo.RunQuery(ctx, query, opts.Limit)
	case r.bigquery != nil:
		return r.bigquery.RunQuery(ctx, query, opts)
	default:
		return r.sql.RunQuery(ctx, query, opts.Limit)
	}
}

func (r *Runner) ListSchema(ctx context.Context, opts QueryOptions) (QueryOutput, error) {
	switch {
	case r.mongo != nil:
		return r.mongo.ListSchema(ctx, opts.Limit)
	case r.bigquery != nil:
		return r.bigquery.ListSchema(ctx, opts)
	default:
		return r.sql.ListSchema(ctx, opts.Limit)
	}
}
