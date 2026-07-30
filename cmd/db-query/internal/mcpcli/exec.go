package mcpcli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Run executes db-query with the given arguments and returns stdout.
// Stderr is included in errors; stdout is never mixed with stderr.
func Run(ctx context.Context, args ...string) ([]byte, error) {
	bin, err := resolveBinary()
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = os.Environ()

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return nil, fmt.Errorf("db-query %s: %w", strings.Join(args, " "), err)
		}
		return nil, fmt.Errorf("db-query %s: %w: %s", strings.Join(args, " "), err, msg)
	}
	return out, nil
}

// ListDatabases returns JSON from db-query -list-dbs -format json.
func ListDatabases(ctx context.Context, configPath string) ([]byte, error) {
	args := []string{"-list-dbs", "-format", "json"}
	if configPath != "" {
		args = append([]string{"-config", configPath}, args...)
	}
	return Run(ctx, args...)
}

// Query runs a read-only query and returns JSON output.
func Query(ctx context.Context, opts QueryOpts) ([]byte, error) {
	args := []string{
		"-db", opts.DB,
		"-query", opts.Query,
		"-format", "json",
		"-limit", fmt.Sprintf("%d", opts.Limit),
	}
	if opts.ConfigPath != "" {
		args = append([]string{"-config", opts.ConfigPath}, args...)
	}
	if opts.Dataset != "" {
		args = append(args, "-dataset", opts.Dataset)
	}
	if opts.Timeout > 0 {
		args = append(args, "-timeout", opts.Timeout.String())
	}
	return Run(ctx, args...)
}

// Ping tests connectivity for a configured database.
func Ping(ctx context.Context, dbName, configPath string) ([]byte, error) {
	args := []string{"-db", dbName, "-ping"}
	if configPath != "" {
		args = append([]string{"-config", configPath}, args...)
	}
	return Run(ctx, args...)
}

// ListCollections returns JSON from db-query -list-collections (MongoDB only).
func ListCollections(ctx context.Context, dbName, configPath string) ([]byte, error) {
	args := []string{"-db", dbName, "-list-collections", "-format", "json"}
	if configPath != "" {
		args = append([]string{"-config", configPath}, args...)
	}
	return Run(ctx, args...)
}

// QueryOpts configures a db-query invocation.
type QueryOpts struct {
	DB         string
	Query      string
	ConfigPath string
	Dataset    string
	Limit      int
	Timeout    time.Duration
}

func resolveBinary() (string, error) {
	if bin := strings.TrimSpace(os.Getenv("DB_QUERY_BIN")); bin != "" {
		return bin, nil
	}

	if exe, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(exe), "db-query")
		if _, err := os.Stat(sibling); err == nil {
			return sibling, nil
		}
	}

	if path, err := exec.LookPath("db-query"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("db-query binary not found: set DB_QUERY_BIN or install db-query on PATH")
}
