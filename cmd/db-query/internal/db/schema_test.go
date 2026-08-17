package db

import (
	"errors"
	"testing"

	"github.com/hydronica/trial"
)

func Test_catalogQueryForDriver(t *testing.T) {
	fn := func(driver string) (bool, error) {
		query, err := catalogQueryForDriver(driver)
		return query != "", err
	}
	cases := trial.Cases[string, bool]{
		"sqlite": {Input: "sqlite", Expected: true},
		"pgx":    {Input: "pgx", Expected: true},
		"mysql":  {Input: "mysql", Expected: true},
		"oracle": {Input: "oracle", Expected: true},
		"unknown driver": {
			Input:       "snowflake",
			ExpectedErr: errors.New("not supported"),
		},
	}
	trial.New(fn, cases).SubTest(t)
}

func Test_sqlTableKind(t *testing.T) {
	fn := func(raw string) (string, error) {
		return sqlTableKind(raw), nil
	}
	cases := trial.Cases[string, string]{
		"base table":         {Input: "BASE TABLE", Expected: "table"},
		"table":              {Input: "TABLE", Expected: "table"},
		"view":               {Input: "VIEW", Expected: "view"},
		"materialized view":  {Input: "MATERIALIZED VIEW", Expected: "materialized_view"},
		"passthrough lower":  {Input: "Collection", Expected: "collection"},
	}
	trial.New(fn, cases).SubTest(t)
}
