package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"

	_ "modernc.org/sqlite"
)

func TestSQLConnectionDetails(t *testing.T) {
	tests := []struct {
		name         string
		dbCfg        config.Database
		wantDriver   string
		wantDSN      string
		wantContains []string
		wantErr      bool
	}{
		{
			name: "mysql builds tcp dsn with default port",
			dbCfg: config.Database{
				Type:     "mysql",
				Host:     "db.example.com",
				DB:       "app",
				Username: "reader",
				Password: "secret",
			},
			wantDriver: "mysql",
			wantDSN:    "reader:secret@tcp(db.example.com:3306)/app",
		},
		{
			name: "mysql preserves explicit host port",
			dbCfg: config.Database{
				Type:     "mysql",
				Host:     "db.example.com:3307",
				DB:       "app",
				Username: "reader",
			},
			wantDriver: "mysql",
			wantContains: []string{
				"reader@tcp(db.example.com:3307)/app",
				"parseTime=true",
			},
		},
		{
			name: "sqlite builds read-only file dsn",
			dbCfg: config.Database{
				Type: "sqlite",
				DB:   "./data/app.sqlite",
			},
			wantDriver: "sqlite",
			wantDSN:    "file:./data/app.sqlite?mode=ro",
		},
		{
			name: "sqlite uses in-memory read-only dsn",
			dbCfg: config.Database{
				Type: "sqlite",
				DB:   ":memory:",
			},
			wantDriver: "sqlite",
			wantDSN:    "file::memory:?mode=ro",
		},
		{
			name: "sqlite uses connection override",
			dbCfg: config.Database{
				Type:       "sqlite",
				DB:         "./ignored.sqlite",
				Connection: "file:/tmp/custom.sqlite?mode=ro&cache=shared",
			},
			wantDriver: "sqlite",
			wantDSN:    "file:/tmp/custom.sqlite?mode=ro&cache=shared",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver, dsn, err := sqlConnectionDetails(&tt.dbCfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("sqlConnectionDetails() = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("sqlConnectionDetails() error = %v", err)
			}
			if driver != tt.wantDriver {
				t.Fatalf("driver = %q, want %q", driver, tt.wantDriver)
			}
			if tt.wantDSN != "" && !strings.HasPrefix(dsn, tt.wantDSN) {
				t.Fatalf("dsn = %q, want prefix %q", dsn, tt.wantDSN)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(dsn, want) {
					t.Fatalf("dsn = %q, want substring %q", dsn, want)
				}
			}
			if tt.dbCfg.Type == "mysql" && tt.wantDSN != "" && !strings.Contains(dsn, "parseTime=true") {
				t.Fatalf("dsn = %q, want parseTime=true", dsn)
			}
		})
	}
}

func TestSQLiteConnectAndQuery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.sqlite")

	setup, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open setup db: %v", err)
	}
	if _, err := setup.Exec(`CREATE TABLE users (id INTEGER, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := setup.Exec(`INSERT INTO users (id, name) VALUES (1, 'alice')`); err != nil {
		t.Fatalf("insert row: %v", err)
	}
	setup.Close()

	runner, err := Connect(ctx, &config.Database{Type: "sqlite", DB: path})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { runner.Close(ctx) })

	output, err := runner.RunQuery(ctx, "SELECT id, name FROM users", QueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("RunQuery() error = %v", err)
	}
	if output.RowCount != 1 {
		t.Fatalf("RowCount = %d, want 1", output.RowCount)
	}
	if got := output.Rows[0]["name"]; got != "alice" {
		t.Fatalf("name = %v, want alice", got)
	}
}
