package config

import (
	"errors"
	"testing"

	"github.com/hydronica/trial"
)

func TestConfig_Validate(t *testing.T) {
	fn := func(cfg Config) (DatabaseConfig, error) {
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		if len(cfg.Databases) == 1 {
			return cfg.Databases[0], nil
		}
		return nil, nil
	}
	cases := trial.Cases[Config, DatabaseConfig]{
		"requires at least one database": {
			Input:       Config{},
			ExpectedErr: errors.New("at least one"),
		},
		"requires database name": {
			Input: Config{
				Databases: DatabaseList{&PostgresConfig{Host: "h", DB: "d", Username: "u"}},
			},
			ExpectedErr: errors.New("name is required"),
		},
		"rejects duplicate names": {
			Input: Config{
				Databases: DatabaseList{
					&OracleConfig{Ident: "a", Connection: "oracle://x"},
					&OracleConfig{Ident: "a", Connection: "oracle://y"},
				},
			},
			ExpectedErr: errors.New(`duplicate database name "a"`),
		},
		"normalizes signal-api bigquery field names": {
			Input: Config{
				Databases: DatabaseList{
					&BigQueryConfig{
						Ident:      "warehouse",
						BQProject: "my-project",
						BQDataset: "analytics",
						BQAuth:    "/path/to/key.json",
					},
				},
			},
			Expected: &BigQueryConfig{
				Ident:        "warehouse",
				Project:     "my-project",
				Dataset:     "analytics",
				Credentials: "/path/to/key.json",
				BQProject:   "my-project",
				BQDataset:   "analytics",
				BQAuth:      "/path/to/key.json",
				Auth:        "service_account",
			},
		},
		"defaults bigquery to adc without credentials": {
			Input: Config{
				Databases: DatabaseList{
					&BigQueryConfig{Ident: "warehouse", Project: "my-project"},
				},
			},
			Expected: &BigQueryConfig{
				Ident:    "warehouse",
				Project: "my-project",
				Auth:    "adc",
			},
		},
		"defaults bigquery to service account when credentials set": {
			Input: Config{
				Databases: DatabaseList{
					&BigQueryConfig{
						Ident:        "warehouse",
						Project:     "my-project",
						Credentials: "/path/to/key.json",
					},
				},
			},
			Expected: &BigQueryConfig{
				Ident:        "warehouse",
				Project:     "my-project",
				Credentials: "/path/to/key.json",
				Auth:        "service_account",
			},
		},
		"accepts explicit adc auth": {
			Input: Config{
				Databases: DatabaseList{
					&BigQueryConfig{
						Ident:        "warehouse",
						Project:     "my-project",
						Auth:        "gcloud",
						Credentials: "/ignored/when-adc.json",
					},
				},
			},
			Expected: &BigQueryConfig{
				Ident:        "warehouse",
				Project:     "my-project",
				Credentials: "/ignored/when-adc.json",
				Auth:        "adc",
			},
		},
		"requires credentials for service account auth": {
			Input: Config{
				Databases: DatabaseList{
					&BigQueryConfig{Ident: "warehouse", Project: "my-project", Auth: "service_account"},
				},
			},
			ExpectedErr: errors.New("requires credentials"),
		},
		"accepts valid postgres entry": {
			Input: Config{
				Databases: DatabaseList{
					&PostgresConfig{Ident: "iap", Host: "h", DB: "iap", Username: "u"},
				},
			},
			Expected: &PostgresConfig{Ident: "iap", Host: "h", DB: "iap", Username: "u"},
		},
		"accepts postgres with connection URI": {
			Input: Config{
				Databases: DatabaseList{
					&PostgresConfig{Ident: "iap", Connection: "postgres://u:p@h/db"},
				},
			},
			Expected: &PostgresConfig{Ident: "iap", Connection: "postgres://u:p@h/db"},
		},
		"requires sslrootcert when ssl skip hostname verify": {
			Input: Config{
				Databases: DatabaseList{
					&PostgresConfig{
						Ident:                  "iap",
						Host:                  "h",
						DB:                    "iap",
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
					&MySQLConfig{Ident: "app", Host: "h", DB: "app", Username: "u"},
				},
			},
			Expected: &MySQLConfig{Ident: "app", Host: "h", DB: "app", Username: "u"},
		},
		"accepts valid sqlite entry with db path": {
			Input: Config{
				Databases: DatabaseList{
					&SQLiteConfig{Ident: "local", DB: "./data/app.sqlite"},
				},
			},
			Expected: &SQLiteConfig{Ident: "local", DB: "./data/app.sqlite"},
		},
		"accepts valid sqlite entry with connection": {
			Input: Config{
				Databases: DatabaseList{
					&SQLiteConfig{Ident: "local", Connection: "file:/tmp/app.sqlite?mode=ro"},
				},
			},
			Expected: &SQLiteConfig{Ident: "local", Connection: "file:/tmp/app.sqlite?mode=ro"},
		},
		"requires sqlite db or connection": {
			Input: Config{
				Databases: DatabaseList{
					&SQLiteConfig{Ident: "local"},
				},
			},
			ExpectedErr: errors.New("sqlite requires db"),
		},
		"accepts valid mongo entry": {
			Input: Config{
				Databases: DatabaseList{
					&MongoConfig{Ident: "reporting", URI: "mongodb://localhost", DBName: "reporting"},
				},
			},
			Expected: &MongoConfig{Ident: "reporting", URI: "mongodb://localhost", DBName: "reporting"},
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
						&OracleConfig{Ident: "only", Connection: "oracle://x"},
					},
				},
			},
			Expected: "only",
		},
		"requires name when multiple configured": {
			Input: findInput{
				cfg: Config{
					Databases: DatabaseList{
						&OracleConfig{Ident: "maindb", Connection: "oracle://x"},
						&PostgresConfig{Ident: "iap", Host: "h", DB: "iap", Username: "u"},
					},
				},
			},
			ExpectedErr: errors.New("database name required"),
		},
		"returns configured database": {
			Input: findInput{
				cfg: Config{
					Databases: DatabaseList{
						&OracleConfig{Ident: "maindb", Connection: "oracle://x"},
						&PostgresConfig{Ident: "iap", Host: "h", DB: "iap", Username: "u"},
					},
				},
				name: "iap",
			},
			Expected: "iap",
		},
		"reports missing database": {
			Input: findInput{
				cfg: Config{
					Databases: DatabaseList{
						&OracleConfig{Ident: "maindb", Connection: "oracle://x"},
						&PostgresConfig{Ident: "iap", Host: "h", DB: "iap", Username: "u"},
					},
				},
				name: "missing",
			},
			ExpectedErr: errors.New(`database "missing" not found`),
		},
	}
	trial.New(fn, cases).SubTest(t)
}
