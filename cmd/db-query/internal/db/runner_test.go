package db

import (
	"errors"
	"testing"

	"github.com/hydronica/trial"
)

func TestValidateReadOnlyQuery(t *testing.T) {
	fn := func(query string) (struct{}, error) {
		return struct{}{}, ValidateReadOnlyQuery(query)
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
		"rejects postgres select into": {
			Input:       "SELECT * INTO new_table FROM users",
			ExpectedErr: errors.New("forbidden keywords"),
		},
		"rejects mysql select into outfile": {
			Input:       "SELECT * INTO OUTFILE '/tmp/users.csv' FROM users",
			ExpectedErr: errors.New("forbidden keywords"),
		},
		"allows into as part of identifier": {
			Input: "SELECT into_count FROM into_items",
		},
		"rejects for update": {
			Input:       "SELECT * FROM users FOR UPDATE",
			ExpectedErr: errors.New("forbidden locking clauses"),
		},
		"rejects with write cte": {
			Input:       "WITH moved AS (DELETE FROM users RETURNING *) SELECT * FROM moved",
			ExpectedErr: errors.New("forbidden keywords"),
		},
	}
	trial.New(fn, cases).SubTest(t)
}
