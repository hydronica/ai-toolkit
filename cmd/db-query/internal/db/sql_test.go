package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"
	"github.com/hydronica/trial"

	_ "modernc.org/sqlite"
)

type sqlConnResult struct {
	Driver   string
	DSN      string
	Contains []string
}

func Test_sqlConnectionDetails(t *testing.T) {
	fn := func(dbCfg config.Database) (sqlConnResult, error) {
		driver, dsn, err := sqlConnectionDetails(&dbCfg)
		if err != nil {
			return sqlConnResult{}, err
		}
		return sqlConnResult{Driver: driver, DSN: dsn}, nil
	}
	cases := trial.Cases[config.Database, sqlConnResult]{
		"mysql builds tcp dsn with default port": {
			Input: config.Database{
				Type:     "mysql",
				Host:     "db.example.com",
				DB:       "app",
				Username: "reader",
				Password: "secret",
			},
			Expected: sqlConnResult{
				Driver: "mysql",
				DSN:    "reader:secret@tcp(db.example.com:3306)/app",
			},
		},
		"mysql preserves explicit host port": {
			Input: config.Database{
				Type:     "mysql",
				Host:     "db.example.com:3307",
				DB:       "app",
				Username: "reader",
			},
			Expected: sqlConnResult{
				Driver: "mysql",
				Contains: []string{
					"reader@tcp(db.example.com:3307)/app",
					"parseTime=true",
				},
			},
		},
		"sqlite builds read-only file dsn": {
			Input: config.Database{
				Type: "sqlite",
				DB:   "./data/app.sqlite",
			},
			Expected: sqlConnResult{
				Driver: "sqlite",
				DSN:    "file:./data/app.sqlite?mode=ro",
			},
		},
		"sqlite uses in-memory read-only dsn": {
			Input: config.Database{
				Type: "sqlite",
				DB:   ":memory:",
			},
			Expected: sqlConnResult{
				Driver: "sqlite",
				DSN:    "file::memory:?mode=ro",
			},
		},
		"sqlite uses connection override": {
			Input: config.Database{
				Type:       "sqlite",
				DB:         "./ignored.sqlite",
				Connection: "file:/tmp/custom.sqlite?mode=ro&cache=shared",
			},
			Expected: sqlConnResult{
				Driver: "sqlite",
				DSN:    "file:/tmp/custom.sqlite?mode=ro&cache=shared",
			},
		},
		"sqlite enforces read only on connection override": {
			Input: config.Database{
				Type:       "sqlite",
				Connection: "file:/tmp/custom.sqlite",
			},
			Expected: sqlConnResult{
				Driver: "sqlite",
				DSN:    "file:/tmp/custom.sqlite?mode=ro",
			},
		},
		"sqlite replaces read write mode on connection override": {
			Input: config.Database{
				Type:       "sqlite",
				Connection: "file:/tmp/custom.sqlite?mode=rw&cache=shared",
			},
			Expected: sqlConnResult{
				Driver: "sqlite",
				DSN:    "file:/tmp/custom.sqlite?mode=ro&cache=shared",
			},
		},
	}
	trial.New(fn, cases).Comparer(func(actual, expected interface{}) (bool, string) {
		got := actual.(sqlConnResult)
		want := expected.(sqlConnResult)
		if got.Driver != want.Driver {
			return false, "driver mismatch"
		}
		if want.DSN != "" && !strings.HasPrefix(got.DSN, want.DSN) {
			return false, "dsn prefix mismatch"
		}
		for _, sub := range want.Contains {
			if !strings.Contains(got.DSN, sub) {
				return false, "dsn missing substring"
			}
		}
		if want.Driver == "mysql" && want.DSN != "" && !strings.Contains(got.DSN, "parseTime=true") {
			return false, "dsn missing parseTime=true"
		}
		return true, ""
	}).SubTest(t)
}

func Test_connectSQL(t *testing.T) {
	fn := func(ctx context.Context) (struct{}, error) {
		_, err := connectSQL(ctx, &config.Database{Type: "sqlite", DB: ":memory:"})
		return struct{}{}, err
	}
	cases := trial.Cases[context.Context, struct{}]{
		"respects cancelled context": {
			Input: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			ExpectedErr: context.Canceled,
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
		t.Fatalf("RunQuery() RowCount = %d, want 1", output.RowCount)
	}
	if got := output.Rows[0]["name"]; got != "alice" {
		t.Fatalf("RunQuery() name = %v, want alice", got)
	}
}
