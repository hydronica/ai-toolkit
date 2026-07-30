package config

import (
	"errors"
	"testing"

	"github.com/hydronica/trial"
)

func TestConfig_Validate(t *testing.T) {
	fn := func(cfg Config) (Database, error) {
		if err := cfg.Validate(); err != nil {
			return Database{}, err
		}
		if len(cfg.Databases) == 1 {
			return cfg.Databases[0], nil
		}
		return Database{}, nil
	}
	cases := trial.Cases[Config, Database]{
		"requires at least one database": {
			Input:       Config{},
			ExpectedErr: errors.New("at least one"),
		},
		"requires database name": {
			Input: Config{
				Databases: []Database{{Type: "postgres", Host: "h", DB: "d", Username: "u"}},
			},
			ExpectedErr: errors.New("name is required"),
		},
		"rejects duplicate names": {
			Input: Config{
				Databases: []Database{
					{Name: "a", Type: "oracle", Connection: "oracle://x"},
					{Name: "a", Type: "oracle", Connection: "oracle://y"},
				},
			},
			ExpectedErr: errors.New(`duplicate database name "a"`),
		},
		"normalizes signal-api bigquery field names": {
			Input: Config{
				Databases: []Database{
					{
						Name:      "warehouse",
						Type:      "bigquery",
						BQProject: "my-project",
						BQDataset: "analytics",
						BQAuth:    "/path/to/key.json",
					},
				},
			},
			Expected: Database{
				Name:        "warehouse",
				Type:        "bigquery",
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
				Databases: []Database{
					{Name: "warehouse", Type: "bigquery", Project: "my-project"},
				},
			},
			Expected: Database{
				Name:    "warehouse",
				Type:    "bigquery",
				Project: "my-project",
				Auth:    "adc",
			},
		},
		"defaults bigquery to service account when credentials set": {
			Input: Config{
				Databases: []Database{
					{
						Name:        "warehouse",
						Type:        "bigquery",
						Project:     "my-project",
						Credentials: "/path/to/key.json",
					},
				},
			},
			Expected: Database{
				Name:        "warehouse",
				Type:        "bigquery",
				Project:     "my-project",
				Credentials: "/path/to/key.json",
				Auth:        "service_account",
			},
		},
		"accepts explicit adc auth": {
			Input: Config{
				Databases: []Database{
					{
						Name:        "warehouse",
						Type:        "bigquery",
						Project:     "my-project",
						Auth:        "gcloud",
						Credentials: "/ignored/when-adc.json",
					},
				},
			},
			Expected: Database{
				Name:        "warehouse",
				Type:        "bigquery",
				Project:     "my-project",
				Credentials: "/ignored/when-adc.json",
				Auth:        "adc",
			},
		},
		"requires credentials for service account auth": {
			Input: Config{
				Databases: []Database{
					{Name: "warehouse", Type: "bigquery", Project: "my-project", Auth: "service_account"},
				},
			},
			ExpectedErr: errors.New("requires credentials"),
		},
		"accepts valid postgres entry": {
			Input: Config{
				Databases: []Database{
					{Name: "iap", Type: "postgres", Host: "h", DB: "iap", Username: "u"},
				},
			},
			Expected: Database{Name: "iap", Type: "postgres", Host: "h", DB: "iap", Username: "u"},
		},
		"accepts valid mysql entry": {
			Input: Config{
				Databases: []Database{
					{Name: "app", Type: "mysql", Host: "h", DB: "app", Username: "u"},
				},
			},
			Expected: Database{Name: "app", Type: "mysql", Host: "h", DB: "app", Username: "u"},
		},
		"accepts valid sqlite entry with db path": {
			Input: Config{
				Databases: []Database{
					{Name: "local", Type: "sqlite", DB: "./data/app.sqlite"},
				},
			},
			Expected: Database{Name: "local", Type: "sqlite", DB: "./data/app.sqlite"},
		},
		"accepts valid sqlite entry with connection": {
			Input: Config{
				Databases: []Database{
					{Name: "local", Type: "sqlite", Connection: "file:/tmp/app.sqlite?mode=ro"},
				},
			},
			Expected: Database{Name: "local", Type: "sqlite", Connection: "file:/tmp/app.sqlite?mode=ro"},
		},
		"requires sqlite db or connection": {
			Input: Config{
				Databases: []Database{
					{Name: "local", Type: "sqlite"},
				},
			},
			ExpectedErr: errors.New("sqlite requires db"),
		},
		"normalizes mongodb type alias": {
			Input: Config{
				Databases: []Database{
					{Name: "reporting", Type: "mongodb", URI: "mongodb://localhost", DBName: "reporting"},
				},
			},
			Expected: Database{Name: "reporting", Type: "mongo", URI: "mongodb://localhost", DBName: "reporting"},
		},
	}
	trial.New(fn, cases).Comparer(func(actual, expected interface{}) (bool, string) {
		got := actual.(Database)
		want := expected.(Database)
		if got.UsesBigQueryADC() != want.UsesBigQueryADC() {
			return false, "UsesBigQueryADC mismatch"
		}
		return trial.Equal(got, want)
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
		return got.Name, nil
	}
	cases := trial.Cases[findInput, string]{
		"selects sole database when name omitted": {
			Input: findInput{
				cfg: Config{
					Databases: []Database{
						{Name: "only", Type: "oracle", Connection: "oracle://x"},
					},
				},
			},
			Expected: "only",
		},
		"requires name when multiple configured": {
			Input: findInput{
				cfg: Config{
					Databases: []Database{
						{Name: "maindb", Type: "oracle", Connection: "oracle://x"},
						{Name: "iap", Type: "postgres", Host: "h", DB: "iap", Username: "u"},
					},
				},
			},
			ExpectedErr: errors.New("database name required"),
		},
		"returns configured database": {
			Input: findInput{
				cfg: Config{
					Databases: []Database{
						{Name: "maindb", Type: "oracle", Connection: "oracle://x"},
						{Name: "iap", Type: "postgres", Host: "h", DB: "iap", Username: "u"},
					},
				},
				name: "iap",
			},
			Expected: "iap",
		},
		"reports missing database": {
			Input: findInput{
				cfg: Config{
					Databases: []Database{
						{Name: "maindb", Type: "oracle", Connection: "oracle://x"},
						{Name: "iap", Type: "postgres", Host: "h", DB: "iap", Username: "u"},
					},
				},
				name: "missing",
			},
			ExpectedErr: errors.New(`database "missing" not found`),
		},
	}
	trial.New(fn, cases).SubTest(t)
}
