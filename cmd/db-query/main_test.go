package main

import (
	"errors"
	"testing"

	"github.com/hydronica/trial"
)

func Test_validateReadOnlyQuery(t *testing.T) {
	fn := func(query string) (struct{}, error) {
		return struct{}{}, validateReadOnlyQuery(query)
	}
	cases := trial.Cases[string, struct{}]{
		"allows select": {
			Input: "SELECT 1 FROM dual",
		},
		"allows with clause": {
			Input: "WITH cte AS (SELECT 1 AS n) SELECT n FROM cte",
		},
		"allows trailing semicolon": {
			Input: "SELECT 1;",
		},
		"rejects insert": {
			Input:       "INSERT INTO users VALUES (1)",
			ExpectedErr: errors.New("read-only SELECT"),
		},
		"rejects multiple statements": {
			Input:       "SELECT 1; SELECT 2",
			ExpectedErr: errors.New("single SQL statement"),
		},
		"rejects forbidden keyword in select": {
			Input:       "SELECT * FROM users WHERE id IN (DELETE FROM archive RETURNING id)",
			ExpectedErr: errors.New("forbidden keywords"),
		},
	}
	trial.New(fn, cases).SubTest(t)
}
