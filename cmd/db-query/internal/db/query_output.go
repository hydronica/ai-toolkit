package db

type QueryOutput struct {
	Columns   []string                   `json:"columns"`
	Rows      []map[string]interface{}   `json:"rows"`
	RowCount  int                        `json:"row_count"`
	Truncated bool                       `json:"truncated,omitempty"`
}
