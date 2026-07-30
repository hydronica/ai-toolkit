package config

import (
	"strings"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name:    "requires at least one database",
			cfg:     Config{},
			wantErr: "at least one",
		},
		{
			name: "requires database name",
			cfg: Config{
				Databases: []Database{{Type: "postgres", Host: "h", DB: "d", Username: "u"}},
			},
			wantErr: "name is required",
		},
		{
			name: "rejects duplicate names",
			cfg: Config{
				Databases: []Database{
					{Name: "a", Type: "oracle", Connection: "oracle://x"},
					{Name: "a", Type: "oracle", Connection: "oracle://y"},
				},
			},
			wantErr: `duplicate database name "a"`,
		},
		{
			name: "normalizes signal-api bigquery field names",
			cfg: Config{
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
		},
		{
			name: "defaults bigquery to adc without credentials",
			cfg: Config{
				Databases: []Database{
					{Name: "warehouse", Type: "bigquery", Project: "my-project"},
				},
			},
		},
		{
			name: "defaults bigquery to service account when credentials set",
			cfg: Config{
				Databases: []Database{
					{
						Name:        "warehouse",
						Type:        "bigquery",
						Project:     "my-project",
						Credentials: "/path/to/key.json",
					},
				},
			},
		},
		{
			name: "accepts explicit adc auth",
			cfg: Config{
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
		},
		{
			name: "requires credentials for service account auth",
			cfg: Config{
				Databases: []Database{
					{Name: "warehouse", Type: "bigquery", Project: "my-project", Auth: "service_account"},
				},
			},
			wantErr: "requires credentials",
		},
		{
			name: "accepts valid postgres entry",
			cfg: Config{
				Databases: []Database{
					{Name: "iap", Type: "postgres", Host: "h", DB: "iap", Username: "u"},
				},
			},
		},
		{
			name: "accepts valid mysql entry",
			cfg: Config{
				Databases: []Database{
					{Name: "app", Type: "mysql", Host: "h", DB: "app", Username: "u"},
				},
			},
		},
		{
			name: "accepts valid sqlite entry with db path",
			cfg: Config{
				Databases: []Database{
					{Name: "local", Type: "sqlite", DB: "./data/app.sqlite"},
				},
			},
		},
		{
			name: "accepts valid sqlite entry with connection",
			cfg: Config{
				Databases: []Database{
					{Name: "local", Type: "sqlite", Connection: "file:/tmp/app.sqlite?mode=ro"},
				},
			},
		},
		{
			name: "requires sqlite db or connection",
			cfg: Config{
				Databases: []Database{
					{Name: "local", Type: "sqlite"},
				},
			},
			wantErr: "sqlite requires db",
		},
		{
			name: "normalizes mongodb type alias",
			cfg: Config{
				Databases: []Database{
					{Name: "reporting", Type: "mongodb", URI: "mongodb://localhost", DBName: "reporting"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				if len(tt.cfg.Databases) == 1 && tt.cfg.Databases[0].Type == "bigquery" && tt.wantErr == "" {
					db := tt.cfg.Databases[0]
					switch tt.name {
					case "normalizes signal-api bigquery field names":
						if db.Project != "my-project" || db.Dataset != "analytics" || db.Credentials != "/path/to/key.json" {
							t.Fatalf("Validate() bigquery fields = project=%q dataset=%q credentials=%q", db.Project, db.Dataset, db.Credentials)
						}
						if db.Auth != "service_account" {
							t.Fatalf("Validate() bigquery auth = %q, want service_account", db.Auth)
						}
					case "defaults bigquery to adc without credentials":
						if db.Auth != "adc" || db.UsesBigQueryADC() != true {
							t.Fatalf("Validate() bigquery auth = %q, want adc", db.Auth)
						}
					case "defaults bigquery to service account when credentials set":
						if db.Auth != "service_account" {
							t.Fatalf("Validate() bigquery auth = %q, want service_account", db.Auth)
						}
					case "accepts explicit adc auth":
						if db.Auth != "adc" || db.UsesBigQueryADC() != true {
							t.Fatalf("Validate() bigquery auth = %q, want adc", db.Auth)
						}
					}
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestConfigFind(t *testing.T) {
	cfg := Config{
		Databases: []Database{
			{Name: "maindb", Type: "oracle", Connection: "oracle://x"},
			{Name: "iap", Type: "postgres", Host: "h", DB: "iap", Username: "u"},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}

	t.Run("selects sole database when name omitted", func(t *testing.T) {
		single := Config{
			Databases: []Database{
				{Name: "only", Type: "oracle", Connection: "oracle://x"},
			},
		}
		if err := single.Validate(); err != nil {
			t.Fatalf("Validate() = %v", err)
		}

		got, err := single.Find("")
		if err != nil {
			t.Fatalf("Find(\"\") error = %v", err)
		}
		if got.Name != "only" {
			t.Fatalf("Find(\"\") name = %q, want %q", got.Name, "only")
		}
	})

	t.Run("requires name when multiple configured", func(t *testing.T) {
		_, err := cfg.Find("")
		if err == nil {
			t.Fatal("Find(\"\") = nil, want error")
		}
	})

	t.Run("returns configured database", func(t *testing.T) {
		got, err := cfg.Find("iap")
		if err != nil {
			t.Fatalf("Find(\"iap\") error = %v", err)
		}
		if got.Type != "postgres" {
			t.Fatalf("Find(\"iap\").Type = %q, want postgres", got.Type)
		}
	})

	t.Run("reports missing database", func(t *testing.T) {
		_, err := cfg.Find("missing")
		if err == nil {
			t.Fatal("Find(\"missing\") = nil, want error")
		}
	})
}
