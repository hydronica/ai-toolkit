package db

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestValidateMongoQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   mongoQuery
		wantErr string
	}{
		{
			name: "allows read-only aggregate",
			query: mongoQuery{
				Collection: "jobs",
				Pipeline:   []bson.M{{"$match": map[string]interface{}{"status": "done"}}},
			},
		},
		{
			name: "rejects write stage $out",
			query: mongoQuery{
				Collection: "jobs",
				Pipeline:   []bson.M{{"$out": "other"}},
			},
			wantErr: `$out`,
		},
		{
			name: "rejects write stage $merge",
			query: mongoQuery{
				Collection: "jobs",
				Pipeline:   []bson.M{{"$merge": "other"}},
			},
			wantErr: `$merge`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMongoQuery(&tt.query)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateMongoQuery() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateMongoQuery() = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateMongoQuery() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestParseMongoQuery(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:  "parses find query",
			input: `{"collection":"Report","filter":{"status":"active"}}`,
		},
		{
			name:    "requires collection",
			input:   `{"filter":{}}`,
			wantErr: "collection",
		},
		{
			name:    "requires json",
			input:   `not json`,
			wantErr: "JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMongoQuery(tt.input)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("parseMongoQuery(%q) error = %v", tt.input, err)
				}
				if got.Collection == "" {
					t.Fatalf("parseMongoQuery(%q) collection = empty, want non-empty", tt.input)
				}
				return
			}
			if err == nil {
				t.Fatalf("parseMongoQuery(%q) = %+v, want error", tt.input, got)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseMongoQuery() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}
