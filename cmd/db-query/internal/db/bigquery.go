package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"
)

type bigqueryRunner struct {
	client   *bigquery.Client
	dataset  string
	location string
}

func connectBigQuery(ctx context.Context, dbCfg *config.Database) (*bigqueryRunner, error) {
	project := strings.TrimSpace(dbCfg.Project)
	if project == "" {
		return nil, fmt.Errorf("bigquery requires project")
	}

	opts, err := bigqueryClientOptions(dbCfg)
	if err != nil {
		return nil, err
	}

	client, err := bigquery.NewClient(ctx, project, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	return &bigqueryRunner{
		client:   client,
		dataset:  strings.TrimSpace(dbCfg.Dataset),
		location: strings.TrimSpace(dbCfg.Location),
	}, nil
}

func bigqueryClientOptions(dbCfg *config.Database) ([]option.ClientOption, error) {
	if dbCfg.UsesBigQueryADC() {
		return nil, nil
	}

	auth := strings.TrimSpace(dbCfg.Credentials)
	if auth == "" {
		return nil, fmt.Errorf(`bigquery auth "service_account" requires credentials (or bq-auth)`)
	}

	return []option.ClientOption{option.WithCredentialsFile(auth)}, nil
}

const bigqueryADCHint = "run `gcloud auth application-default login` or set credentials/bq-auth for a service account"

func (r *bigqueryRunner) applyDefaults(q *bigquery.Query, dataset string) {
	if dataset == "" {
		dataset = r.dataset
	}
	if dataset != "" {
		q.DefaultProjectID = r.client.Project()
		q.DefaultDatasetID = dataset
	}
	if r.location != "" {
		q.Location = r.location
	}
}

func (r *bigqueryRunner) Ping(ctx context.Context, opts QueryOptions) error {
	q := r.client.Query("SELECT 1")
	q.DryRun = true
	r.applyDefaults(q, opts.Dataset)

	job, err := q.Run(ctx)
	if err != nil {
		return fmt.Errorf("ping: %w; %s", err, bigqueryADCHint)
	}
	if status := job.LastStatus(); status != nil && status.Err() != nil {
		return fmt.Errorf("ping: %w; %s", status.Err(), bigqueryADCHint)
	}
	return nil
}

func (r *bigqueryRunner) Close() error {
	return r.client.Close()
}

func (r *bigqueryRunner) RunQuery(ctx context.Context, query string, opts QueryOptions) (QueryOutput, error) {
	bqQuery := r.client.Query(query)
	r.applyDefaults(bqQuery, opts.Dataset)

	job, err := bqQuery.Run(ctx)
	if err != nil {
		return QueryOutput{}, fmt.Errorf("query: %w", err)
	}

	status, err := job.Wait(ctx)
	if err != nil {
		return QueryOutput{}, fmt.Errorf("wait: %w", err)
	}
	if status.Err() != nil {
		return QueryOutput{}, fmt.Errorf("query: %w", status.Err())
	}

	it, err := job.Read(ctx)
	if err != nil {
		return QueryOutput{}, fmt.Errorf("read: %w", err)
	}

	output := QueryOutput{
		Columns: schemaColumnNames(it.Schema),
		Rows:    make([]map[string]interface{}, 0),
	}

	limit := opts.Limit
	for {
		if limit > 0 && output.RowCount >= limit {
			output.Truncated = true
			break
		}

		var row []bigquery.Value
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return QueryOutput{}, fmt.Errorf("next row: %w", err)
		}

		record := make(map[string]interface{}, len(it.Schema))
		for i, field := range it.Schema {
			if i >= len(row) {
				break
			}
			record[field.Name] = normalizeBigQueryValue(row[i])
		}

		output.Rows = append(output.Rows, record)
		output.RowCount++
	}

	if len(output.Columns) == 0 && len(output.Rows) > 0 {
		output.Columns = sortedKeys(output.Rows[0])
	}

	return output, nil
}

func schemaColumnNames(schema bigquery.Schema) []string {
	columns := make([]string, len(schema))
	for i, field := range schema {
		columns[i] = field.Name
	}
	return columns
}

func normalizeBigQueryValue(value bigquery.Value) interface{} {
	switch v := value.(type) {
	case nil:
		return nil
	case time.Time:
		return v.UTC().Format(time.RFC3339Nano)
	case []bigquery.Value:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = normalizeBigQueryValue(item)
		}
		return out
	default:
		return v
	}
}
