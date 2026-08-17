package config

import (
	"errors"
	"testing"

	"github.com/hydronica/trial"
)

func TestConfig_Validate(t *testing.T) {
	fn := func(cfg Config) (Database, error) {
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		if len(cfg.Databases) == 1 {
			return cfg.Databases[0], nil
		}
		return nil, nil
	}
	cases := trial.Cases[Config, Database]{
		"requires at least one database": {
			Input:       Config{},
			ExpectedErr: errors.New("at least one"),
		},
		"requires database name": {
			Input: Config{
				Databases: DatabaseList{&Postgres{Host: "h", DB: "d", Username: "u"}},
			},
			ExpectedErr: errors.New("name is required"),
		},
		"rejects duplicate names": {
			Input: Config{
				Databases: DatabaseList{
					&Oracle{ID: "a", Connection: "oracle://x"},
					&Oracle{ID: "a", Connection: "oracle://y"},
				},
			},
			ExpectedErr: errors.New(`duplicate database name "a"`),
		},
		"defaults bigquery to adc without credentials": {
			Input: Config{
				Databases: DatabaseList{
					&BigQuery{ID: "warehouse", Project: "my-project"},
				},
			},
			Expected: &BigQuery{
				ID:      "warehouse",
				Project: "my-project",
				Auth:    "adc",
			},
		},
		"defaults bigquery to service account when credentials set": {
			Input: Config{
				Databases: DatabaseList{
					&BigQuery{
						ID:          "warehouse",
						Project:     "my-project",
						Credentials: "/path/to/key.json",
					},
				},
			},
			Expected: &BigQuery{
				ID:          "warehouse",
				Project:     "my-project",
				Credentials: "/path/to/key.json",
				Auth:        "service_account",
			},
		},
		"accepts explicit adc auth": {
			Input: Config{
				Databases: DatabaseList{
					&BigQuery{
						ID:          "warehouse",
						Project:     "my-project",
						Auth:        "gcloud",
						Credentials: "/ignored/when-adc.json",
					},
				},
			},
			Expected: &BigQuery{
				ID:          "warehouse",
				Project:     "my-project",
				Credentials: "/ignored/when-adc.json",
				Auth:        "adc",
			},
		},
		"requires credentials for service account auth": {
			Input: Config{
				Databases: DatabaseList{
					&BigQuery{ID: "warehouse", Project: "my-project", Auth: "service_account"},
				},
			},
			ExpectedErr: errors.New("requires credentials"),
		},
		"accepts valid postgres entry": {
			Input: Config{
				Databases: DatabaseList{
					&Postgres{ID: "postgres", Host: "h", DB: "postgres", Username: "u"},
				},
			},
			Expected: &Postgres{ID: "postgres", Host: "h", DB: "postgres", Username: "u"},
		},
		"accepts postgres with connection URI": {
			Input: Config{
				Databases: DatabaseList{
					&Postgres{ID: "postgres", Connection: "postgres://u:p@h/db"},
				},
			},
			Expected: &Postgres{ID: "postgres", Connection: "postgres://u:p@h/db"},
		},
		"requires sslrootcert when ssl skip hostname verify": {
			Input: Config{
				Databases: DatabaseList{
					&Postgres{
						ID:                    "postgres",
						Host:                  "h",
						DB:                    "postgres",
						Username:              "u",
						SSLSkipHostnameVerify: true,
					},
				},
			},
			ExpectedErr: errors.New("ssl_skip_hostname_verify requires sslrootcert"),
		},
		"accepts valid mysql entry": {
			Input: Config{
				Databases: DatabaseList{
					&MySQL{ID: "app", Host: "h", DB: "app", Username: "u"},
				},
			},
			Expected: &MySQL{ID: "app", Host: "h", DB: "app", Username: "u"},
		},
		"accepts valid sqlite entry with db path": {
			Input: Config{
				Databases: DatabaseList{
					&SQLite{ID: "local", DB: "./data/app.sqlite"},
				},
			},
			Expected: &SQLite{ID: "local", DB: "./data/app.sqlite"},
		},
		"accepts valid sqlite entry with connection": {
			Input: Config{
				Databases: DatabaseList{
					&SQLite{ID: "local", Connection: "file:/tmp/app.sqlite?mode=ro"},
				},
			},
			Expected: &SQLite{ID: "local", Connection: "file:/tmp/app.sqlite?mode=ro"},
		},
		"requires sqlite db or connection": {
			Input: Config{
				Databases: DatabaseList{
					&SQLite{ID: "local"},
				},
			},
			ExpectedErr: errors.New("sqlite requires db"),
		},
		"accepts valid mongo entry": {
			Input: Config{
				Databases: DatabaseList{
					&Mongo{ID: "reporting", URI: "mongodb://localhost", DBName: "reporting"},
				},
			},
			Expected: &Mongo{ID: "reporting", URI: "mongodb://localhost", DBName: "reporting"},
		},
	}
	trial.New(fn, cases).Comparer(func(actual, expected interface{}) (bool, string) {
		if actual == nil && expected == nil {
			return true, ""
		}
		return trial.Equal(actual, expected)
	}).SubTest(t)
}

type findInput struct {
	cfg  Config
	name string
}

func TestConfig_Find(t *testing.T) {
	fn := func(in findInput) (string, error) {
		if err := in.cfg.Validate(); err != nil {
			return "", err
		}
		got, err := in.cfg.Find(in.name)
		if err != nil {
			return "", err
		}
		return got.Name(), nil
	}
	cases := trial.Cases[findInput, string]{
		"selects sole database when name omitted": {
			Input: findInput{
				cfg: Config{
					Databases: DatabaseList{
						&Oracle{ID: "only", Connection: "oracle://x"},
					},
				},
			},
			Expected: "only",
		},
		"requires name when multiple configured": {
			Input: findInput{
				cfg: Config{
					Databases: DatabaseList{
						&Oracle{ID: "oracle", Connection: "oracle://x"},
						&Postgres{ID: "postgres", Host: "h", DB: "postgres", Username: "u"},
					},
				},
			},
			ExpectedErr: errors.New("database name required"),
		},
		"returns configured database": {
			Input: findInput{
				cfg: Config{
					Databases: DatabaseList{
						&Oracle{ID: "oracle", Connection: "oracle://x"},
						&Postgres{ID: "postgres", Host: "h", DB: "postgres", Username: "u"},
					},
				},
				name: "postgres",
			},
			Expected: "postgres",
		},
		"reports missing database": {
			Input: findInput{
				cfg: Config{
					Databases: DatabaseList{
						&Oracle{ID: "oracle", Connection: "oracle://x"},
						&Postgres{ID: "postgres", Host: "h", DB: "postgres", Username: "u"},
					},
				},
				name: "missing",
			},
			ExpectedErr: errors.New(`database "missing" not found`),
		},
	}
	trial.New(fn, cases).SubTest(t)
}
