package db

import (
	"errors"
	"testing"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"
	"github.com/hydronica/trial"
)

func Test_postgresTLSConfigSkipHostname(t *testing.T) {
	fn := func(dbCfg config.Database) (struct{}, error) {
		_, err := postgresTLSConfigSkipHostname(&dbCfg)
		return struct{}{}, err
	}
	cases := trial.Cases[config.Database, struct{}]{
		"requires sslrootcert": {
			Input: config.Database{
				SSLSkipHostnameVerify: true,
			},
			ExpectedErr: errors.New("sslrootcert is required"),
		},
	}
	trial.New(fn, cases).SubTest(t)
}
