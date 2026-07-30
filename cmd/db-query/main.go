package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"
	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/db"
)

var forbidden = regexp.MustCompile(`\b(INSERT|UPDATE|DELETE|DROP|ALTER|CREATE|TRUNCATE|MERGE|GRANT|REVOKE|EXEC|EXECUTE|CALL|COPY|REPLACE|RENAME|INTO)\b`)

var readOnlySideEffects = regexp.MustCompile(`\b(FOR\s+UPDATE|FOR\s+SHARE|LOCK\s+IN\s+SHARE\s+MODE)\b`)

func main() {
	queryFlag := flag.String("query", "", "SQL SELECT query to execute")
	configFlag := flag.String("config", config.DefaultPath(), "Path to TOML config file")
	dbFlag := flag.String("db", "", "Database name from config (required when multiple databases are configured)")
	listDBs := flag.Bool("list-dbs", false, "List configured databases and exit")
	datasetFlag := flag.String("dataset", "", "BigQuery only: default dataset for unqualified table names (optional; use `project.dataset.table` in SQL instead)")
	format := flag.String("format", "json", "Output format: json, csv, table")
	limit := flag.Int("limit", 1000, "Maximum rows to return (0 = unlimited)")
	timeout := flag.Duration("timeout", 5*time.Minute, "Query timeout")
	ping := flag.Bool("ping", false, "Test database connection and exit")
	listCollections := flag.Bool("list-collections", false, "MongoDB only: list collections in the configured database and exit")
	flag.Parse()

	cfg, err := config.Load(*configFlag)
	if err != nil {
		exitErr(err)
	}

	if *listDBs {
		if strings.EqualFold(*format, "json") {
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

	dbCfg, err := cfg.Find(*dbFlag)
	if err != nil {
		exitErr(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	queryOpts := db.QueryOptions{
		Limit:   *limit,
		Dataset: strings.TrimSpace(*datasetFlag),
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

	if *ping {
		if err := runner.Ping(ctx, queryOpts); err != nil {
			exitErr(err)
		}
		fmt.Fprintf(os.Stderr, "connected to %s (%s)\n", dbCfg.Name, dbCfg.Type)
		return
	}

	if *listCollections {
		if dbCfg.Type != "mongo" {
			exitErr(errors.New("-list-collections requires a mongo database"))
		}
		output, err := runner.ListCollections(ctx)
		if err != nil {
			exitErr(err)
		}
		if err := writeOutput(os.Stdout, output, *format); err != nil {
			exitErr(err)
		}
		return
	}

	query, err := readQuery(*queryFlag)
	if err != nil {
		exitErr(err)
	}

	if dbCfg.Type != "mongo" {
		if err := validateReadOnlyQuery(query); err != nil {
			exitErr(err)
		}
	}

	output, err := runner.RunQuery(ctx, query, queryOpts)
	if err != nil {
		exitErr(err)
	}

	if err := writeOutput(os.Stdout, output, *format); err != nil {
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

func validateReadOnlyQuery(query string) error {
	trimmed := strings.TrimSpace(query)
	trimmed = strings.TrimSuffix(trimmed, ";")
	upper := strings.ToUpper(trimmed)

	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		return errors.New("only read-only SELECT queries are allowed")
	}
	if readOnlySideEffects.MatchString(upper) {
		return errors.New("query contains forbidden locking clauses; only read-only SELECT queries are allowed")
	}
	if forbidden.MatchString(upper) {
		return errors.New("query contains forbidden keywords; only read-only SELECT queries are allowed")
	}
	if strings.Count(trimmed, ";") > 0 {
		return errors.New("only a single SQL statement is allowed")
	}
	return nil
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
