package db

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"
)

func connectPostgres(ctx context.Context, dbCfg *config.Database) (*sqlRunner, error) {
	if !dbCfg.SSLSkipHostnameVerify {
		return connectSQL(ctx, dbCfg)
	}

	tlsConfig, err := postgresTLSConfigSkipHostname(dbCfg)
	if err != nil {
		return nil, err
	}

	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s/%s?connect_timeout=5",
		url.QueryEscape(dbCfg.Username),
		url.QueryEscape(dbCfg.Password),
		dbCfg.Host,
		dbCfg.DB,
	)

	pgxCfg, err := pgx.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	pgxCfg.TLSConfig = tlsConfig

	db := stdlib.OpenDB(*pgxCfg)
	sqlxDB := sqlx.NewDb(db, "pgx")

	if err := sqlxDB.PingContext(ctx); err != nil {
		sqlxDB.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return &sqlRunner{db: sqlxDB}, nil
}

func postgresTLSConfigSkipHostname(dbCfg *config.Database) (*tls.Config, error) {
	if strings.TrimSpace(dbCfg.SSLRootcert) == "" {
		return nil, errors.New("sslrootcert is required when ssl_skip_hostname_verify is true")
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if dbCfg.SSLCert != "" && dbCfg.SSLKey != "" {
		cert, err := tls.LoadX509KeyPair(dbCfg.SSLCert, dbCfg.SSLKey)
		if err != nil {
			return nil, fmt.Errorf("load client cert: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	roots := x509.NewCertPool()
	pem, err := os.ReadFile(dbCfg.SSLRootcert)
	if err != nil {
		return nil, fmt.Errorf("read sslrootcert: %w", err)
	}
	if !roots.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("parse sslrootcert")
	}
	tlsConfig.RootCAs = roots

	// Verify the chain against the CA but skip hostname matching.
	tlsConfig.InsecureSkipVerify = true
	tlsConfig.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		return verifyPeerCertChain(rawCerts, roots)
	}

	return tlsConfig, nil
}

func verifyPeerCertChain(rawCerts [][]byte, roots *x509.CertPool) error {
	if len(rawCerts) == 0 {
		return fmt.Errorf("no server certificate")
	}
	cert, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return fmt.Errorf("parse server cert: %w", err)
	}
	intermediates := x509.NewCertPool()
	for _, raw := range rawCerts[1:] {
		intermediate, err := x509.ParseCertificate(raw)
		if err != nil {
			return fmt.Errorf("parse intermediate cert: %w", err)
		}
		intermediates.AddCert(intermediate)
	}
	_, err = cert.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
	})
	if err != nil {
		return fmt.Errorf("verify server cert: %w", err)
	}
	return nil
}
