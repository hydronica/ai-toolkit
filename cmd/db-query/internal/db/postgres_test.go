package db

import (
	"errors"
	"testing"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"
	"github.com/hydronica/trial"
)

func Test_postgresTLSConfigSkipHostname(t *testing.T) {
	fn := func(cfg config.Postgres) (struct{}, error) {
		_, err := postgresTLSConfigSkipHostname(&cfg)
		return struct{}{}, err
	}
	cases := trial.Cases[config.Postgres, struct{}]{
		"requires sslrootcert": {
			Input: config.Postgres{
				SSLSkipHostnameVerify: true,
			},
			ExpectedErr: errors.New("sslrootcert is required"),
		},
	}
	trial.New(fn, cases).SubTest(t)
}
