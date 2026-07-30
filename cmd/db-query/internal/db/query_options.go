package db

// QueryOptions controls per-query execution. Dataset applies to BigQuery only.
type QueryOptions struct {
	Limit   int
	Dataset string
}
