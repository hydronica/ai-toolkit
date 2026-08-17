package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	goconfig "github.com/hydronica/go-config"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"
	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/db"
)

var version = "dev"

func main() {
	cfg := &config.Config{
		Format:  "json",
		Limit:   1000,
		Timeout: 5 * time.Minute,
	}
	if err := goconfig.New(cfg).
		ConfigPath(config.DefaultPath()).
		Version(version).
		Description("Run read-only queries against configured databases.").
		Load(); err != nil {
		exitErr(err)
	}

	if cfg.MCP {
		serveMCP(cfg)
		return
	}

	if cfg.ListDBs {
		if strings.EqualFold(cfg.Format, "json") {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(cfg.Summaries()); err != nil {
				exitErr(err)
			}
			return
		}
		for _, name := range cfg.Names() {
			fmt.Println(name)
		}
		return
	}

	dbCfg, err := cfg.Find(cfg.DB)
	if err != nil {
		exitErr(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	queryOpts := db.QueryOptions{
		Limit:   cfg.Limit,
		Dataset: strings.TrimSpace(cfg.Dataset),
	}

	runner, err := db.Connect(ctx, dbCfg)
	if err != nil {
		exitErr(err)
	}
	defer func() {
		if err := runner.Close(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "warning: close: %v\n", err)
		}
	}()

	if cfg.Ping {
		if err := runner.Ping(ctx, queryOpts); err != nil {
			exitErr(err)
		}
		fmt.Fprintf(os.Stderr, "connected to %s (%s)\n", dbCfg.Name(), dbCfg.Type())
		return
	}

	if cfg.ListSchema || cfg.ListCollections {
		output, err := runner.ListSchema(ctx, queryOpts)
		if err != nil {
			exitErr(err)
		}
		if err := writeOutput(os.Stdout, output, cfg.Format); err != nil {
			exitErr(err)
		}
		return
	}

	query, err := readQuery(cfg.Query)
	if err != nil {
		exitErr(err)
	}

	output, err := runner.RunQuery(ctx, query, queryOpts)
	if err != nil {
		exitErr(err)
	}

	if err := writeOutput(os.Stdout, output, cfg.Format); err != nil {
		exitErr(err)
	}
}

func readQuery(flagQuery string) (string, error) {
	if strings.TrimSpace(flagQuery) != "" {
		return strings.TrimSpace(flagQuery), nil
	}

	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("stdin: %w", err)
	}
	if stat.Mode()&os.ModeCharDevice != 0 {
		return "", errors.New("query required: pass -query or pipe SQL via stdin")
	}

	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	query := strings.TrimSpace(string(body))
	if query == "" {
		return "", errors.New("query is empty")
	}
	return query, nil
}


func writeOutput(w io.Writer, output db.QueryOutput, format string) error {
	switch strings.ToLower(format) {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	case "csv":
		writer := csv.NewWriter(w)
		defer writer.Flush()

		if err := writer.Write(output.Columns); err != nil {
			return err
		}
		for _, row := range output.Rows {
			record := make([]string, len(output.Columns))
			for i, col := range output.Columns {
				record[i] = fmt.Sprint(row[col])
			}
			if err := writer.Write(record); err != nil {
				return err
			}
		}
		return writer.Error()
	case "table":
		return writeTable(w, output)
	default:
		return fmt.Errorf("unknown format %q (use json, csv, or table)", format)
	}
}

func writeTable(w io.Writer, output db.QueryOutput) error {
	widths := make([]int, len(output.Columns))
	for i, col := range output.Columns {
		widths[i] = len(col)
	}
	for _, row := range output.Rows {
		for i, col := range output.Columns {
			width := len(fmt.Sprint(row[col]))
			if width > widths[i] {
				widths[i] = width
			}
		}
	}

	writeRow := func(values []string) {
		for i, value := range values {
			if i > 0 {
				fmt.Fprint(w, "  ")
			}
			fmt.Fprintf(w, "%-*s", widths[i], value)
		}
		fmt.Fprintln(w)
	}

	header := make([]string, len(output.Columns))
	copy(header, output.Columns)
	writeRow(header)

	separator := make([]string, len(output.Columns))
	for i, width := range widths {
		separator[i] = strings.Repeat("-", width)
	}
	writeRow(separator)

	for _, row := range output.Rows {
		record := make([]string, len(output.Columns))
		for i, col := range output.Columns {
			record[i] = fmt.Sprint(row[col])
		}
		writeRow(record)
	}

	if output.Truncated {
		fmt.Fprintf(w, "\n(showing first %d rows)\n", output.RowCount)
	}
	return nil
}

func exitErr(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
