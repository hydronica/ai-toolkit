package db

import (
	"errors"
	"testing"

	"github.com/hydronica/trial"
	"go.mongodb.org/mongo-driver/bson"
)

func Test_validateMongoQuery(t *testing.T) {
	fn := func(query mongoQuery) (struct{}, error) {
		return struct{}{}, validateMongoQuery(&query)
	}
	cases := trial.Cases[mongoQuery, struct{}]{
		"allows read-only aggregate": {
			Input: mongoQuery{
				Collection: "jobs",
				Pipeline:   []bson.M{{"$match": map[string]interface{}{"status": "done"}}},
			},
		},
		"rejects write stage $out": {
			Input: mongoQuery{
				Collection: "jobs",
				Pipeline:   []bson.M{{"$out": "other"}},
			},
			ExpectedErr: errors.New(`$out`),
		},
		"rejects write stage $merge": {
			Input: mongoQuery{
				Collection: "jobs",
				Pipeline:   []bson.M{{"$merge": "other"}},
			},
			ExpectedErr: errors.New(`$merge`),
		},
	}
	trial.New(fn, cases).SubTest(t)
}

func Test_parseMongoQuery(t *testing.T) {
	fn := func(input string) (string, error) {
		got, err := parseMongoQuery(input)
		if err != nil {
			return "", err
		}
		return got.Collection, nil
	}
	cases := trial.Cases[string, string]{
		"parses find query": {
			Input:    `{"collection":"Report","filter":{"status":"active"}}`,
			Expected: "Report",
		},
		"requires collection": {
			Input:       `{"filter":{}}`,
			ExpectedErr: errors.New("collection"),
		},
		"requires json": {
			Input:       `not json`,
			ExpectedErr: errors.New("JSON"),
		},
	}
	trial.New(fn, cases).SubTest(t)
}
