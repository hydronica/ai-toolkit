package main

import (
	"strings"
	"testing"
)

func TestValidateReadOnlyQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr string
	}{
		{
			name:  "allows select",
			query: "SELECT 1 FROM dual",
		},
		{
			name:  "allows with clause",
			query: "WITH cte AS (SELECT 1 AS n) SELECT n FROM cte",
		},
		{
			name:  "allows trailing semicolon",
			query: "SELECT 1;",
		},
		{
			name:    "rejects insert",
			query:   "INSERT INTO users VALUES (1)",
			wantErr: "read-only SELECT",
		},
		{
			name:    "rejects multiple statements",
			query:   "SELECT 1; SELECT 2",
			wantErr: "single SQL statement",
		},
		{
			name:    "rejects forbidden keyword in select",
			query:   "SELECT * FROM users WHERE id IN (DELETE FROM archive RETURNING id)",
			wantErr: "forbidden keywords",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReadOnlyQuery(tt.query)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateReadOnlyQuery(%q) error = %v, want nil", tt.query, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateReadOnlyQuery(%q) = nil, want error containing %q", tt.query, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateReadOnlyQuery() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}
