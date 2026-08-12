package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"
	"github.com/hydronica/trial"

	_ "modernc.org/sqlite"
)

func TestMySQLConfig_DSN(t *testing.T) {
	fn := func(cfg config.MySQLConfig) (string, error) {
		return cfg.DSN(), nil
	}
	cases := trial.Cases[config.MySQLConfig, string]{
		"builds tcp dsn with default port": {
			Input: config.MySQLConfig{
				Host:     "db.example.com",
				DB:       "app",
				Username: "reader",
				Password: "secret",
			},
			Expected: "reader:secret@tcp(db.example.com:3306)/app?checkConnLiveness=false&parseTime=true&timeout=5s&maxAllowedPacket=0",
		},
		"preserves explicit host port": {
			Input: config.MySQLConfig{
				Host:     "db.example.com:3307",
				DB:       "app",
				Username: "reader",
			},
			Expected: "reader@tcp(db.example.com:3307)/app?checkConnLiveness=false&parseTime=true&timeout=5s&maxAllowedPacket=0",
		},
		"uses connection DSN when set": {
			Input: config.MySQLConfig{
				Connection: "user:pass@tcp(host:3306)/mydb?parseTime=true",
				Host:       "ignored",
				Username:   "ignored",
				DB:         "ignored",
			},
			Expected: "user:pass@tcp(host:3306)/mydb?parseTime=true",
		},
	}
	trial.New(fn, cases).SubTest(t)
}

func TestSQLiteConfig_DSN(t *testing.T) {
	fn := func(cfg config.SQLiteConfig) (string, error) {
		return cfg.DSN(), nil
	}
	cases := trial.Cases[config.SQLiteConfig, string]{
		"builds read-only file dsn": {
			Input:    config.SQLiteConfig{DB: "./data/app.sqlite"},
			Expected: "file:./data/app.sqlite?mode=ro",
		},
		"in-memory skips read-only": {
			Input:    config.SQLiteConfig{DB: ":memory:"},
			Expected: "file::memory:",
		},
		"uses connection override": {
			Input:    config.SQLiteConfig{DB: "./ignored.sqlite", Connection: "file:/tmp/custom.sqlite?mode=ro&cache=shared"},
			Expected: "file:/tmp/custom.sqlite?mode=ro&cache=shared",
		},
		"enforces read only on connection override": {
			Input:    config.SQLiteConfig{Connection: "file:/tmp/custom.sqlite"},
			Expected: "file:/tmp/custom.sqlite?mode=ro",
		},
		"replaces read write mode on connection override": {
			Input:    config.SQLiteConfig{Connection: "file:/tmp/custom.sqlite?mode=rw&cache=shared"},
			Expected: "file:/tmp/custom.sqlite?mode=ro&cache=shared",
		},
	}
	trial.New(fn, cases).SubTest(t)
}

func TestConnect_SQLite(t *testing.T) {
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

	runner, err := Connect(ctx, &config.SQLiteConfig{Name: "test", DB: path})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { runner.Close(ctx) })

	output, err := runner.RunQuery(ctx, "SELECT id, name FROM users", QueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("RunQuery() error = %v", err)
	}
	if output.RowCount != 1 {
		t.Fatalf("RunQuery() RowCount = %d, want 1", output.RowCount)
	}
	if got := output.Rows[0]["name"]; got != "alice" {
		t.Fatalf("RunQuery() name = %v, want alice", got)
	}
}
