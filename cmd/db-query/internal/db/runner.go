package db

import (
	"context"
	"errors"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"
)

type Runner struct {
	sql       *sqlRunner
	bigquery  *bigqueryRunner
	mongo     *mongoRunner
}

func Connect(ctx context.Context, dbCfg *config.Database) (*Runner, error) {
	switch dbCfg.Type {
	case "mongo":
		mongoRunner, err := connectMongo(ctx, dbCfg)
		if err != nil {
			return nil, err
		}
		return &Runner{mongo: mongoRunner}, nil
	case "bigquery":
		bqRunner, err := connectBigQuery(ctx, dbCfg)
		if err != nil {
			return nil, err
		}
		return &Runner{bigquery: bqRunner}, nil
	case "postgres":
		pgRunner, err := connectPostgres(ctx, dbCfg)
		if err != nil {
			return nil, err
		}
		return &Runner{sql: pgRunner}, nil
	default:
		sqlRunner, err := connectSQL(ctx, dbCfg)
		if err != nil {
			return nil, err
		}
		return &Runner{sql: sqlRunner}, nil
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
	switch {
	case r.mongo != nil:
		return r.mongo.RunQuery(ctx, query, opts.Limit)
	case r.bigquery != nil:
		return r.bigquery.RunQuery(ctx, query, opts)
	default:
		return r.sql.RunQuery(ctx, query, opts.Limit)
	}
}

func (r *Runner) ListCollections(ctx context.Context) (QueryOutput, error) {
	if r.mongo == nil {
		return QueryOutput{}, errors.New("-list-collections is only supported for mongo databases")
	}
	return r.mongo.ListCollections(ctx)
}
