package db

import (
	"errors"
	"testing"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"
	"github.com/hydronica/trial"
)

func Test_postgresTLSConfigSkipHostname(t *testing.T) {
	fn := func(cfg config.PostgresConfig) (struct{}, error) {
		_, err := postgresTLSConfigSkipHostname(&cfg)
		return struct{}{}, err
	}
	cases := trial.Cases[config.PostgresConfig, struct{}]{
		"requires sslrootcert": {
			Input: config.PostgresConfig{
				SSLSkipHostnameVerify: true,
			},
			ExpectedErr: errors.New("sslrootcert is required"),
		},
	}
	trial.New(fn, cases).SubTest(t)
}
