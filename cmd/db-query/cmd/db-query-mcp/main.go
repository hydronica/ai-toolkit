package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/mcpcli"
)

const defaultQueryLimit = 1000

func main() {
	s := server.NewMCPServer(
		"db-query",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	s.AddTool(listDatabasesTool(), handleListDatabases)
	s.AddTool(runQueryTool(), handleRunQuery)
	s.AddTool(pingDatabaseTool(), handlePingDatabase)
	s.AddTool(listCollectionsTool(), handleListCollections)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "db-query-mcp: %v\n", err)
		os.Exit(1)
	}
}

func listDatabasesTool() mcp.Tool {
	return mcp.NewTool("list_databases",
		mcp.WithDescription("List databases configured in db-query (name and type). Call this before run_query to pick a valid -db value."),
		mcp.WithString("config",
			mcp.Description("Optional path to config.toml (defaults to config.toml beside the binary)"),
		),
	)
}

func runQueryTool() mcp.Tool {
	return mcp.NewTool("run_query",
		mcp.WithDescription("Run a read-only SQL or MongoDB query against a configured database. Returns JSON with columns, rows, row_count, and truncated."),
		mcp.WithString("db",
			mcp.Required(),
			mcp.Description("Database name from list_databases (e.g. iap, warehouse, maindb)"),
		),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("SQL SELECT or MongoDB find/aggregate JSON"),
		),
		mcp.WithString("config",
			mcp.Description("Optional path to config.toml"),
		),
		mcp.WithString("dataset",
			mcp.Description("BigQuery only: default dataset for unqualified table names"),
		),
		mcp.WithNumber("limit",
			mcp.Description(fmt.Sprintf("Maximum rows to return (default %d; 0 = unlimited)", defaultQueryLimit)),
		),
		mcp.WithNumber("timeout_seconds",
			mcp.Description("Query timeout in seconds (default 300)"),
		),
	)
}

func pingDatabaseTool() mcp.Tool {
	return mcp.NewTool("ping_database",
		mcp.WithDescription("Test connectivity to a configured database without running a query."),
		mcp.WithString("db",
			mcp.Required(),
			mcp.Description("Database name from list_databases"),
		),
		mcp.WithString("config",
			mcp.Description("Optional path to config.toml"),
		),
	)
}

func listCollectionsTool() mcp.Tool {
	return mcp.NewTool("list_collections",
		mcp.WithDescription("List collections and views in a MongoDB database configured in db-query. Returns JSON with columns name and type."),
		mcp.WithString("db",
			mcp.Required(),
			mcp.Description("MongoDB database name from list_databases"),
		),
		mcp.WithString("config",
			mcp.Description("Optional path to config.toml"),
		),
	)
}

func handleListDatabases(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	configPath := req.GetString("config", "")
	out, err := mcpcli.ListDatabases(ctx, configPath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

func handleRunQuery(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dbName, err := req.RequireString("db")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	configPath := req.GetString("config", "")
	dataset := req.GetString("dataset", "")

	limit := defaultQueryLimit
	if raw, ok := req.GetArguments()["limit"]; ok && raw != nil {
		switch v := raw.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		}
	}

	timeout := 5 * time.Minute
	if raw, ok := req.GetArguments()["timeout_seconds"]; ok && raw != nil {
		switch v := raw.(type) {
		case float64:
			timeout = time.Duration(v) * time.Second
		case int:
			timeout = time.Duration(v) * time.Second
		}
	}

	out, err := mcpcli.Query(ctx, mcpcli.QueryOpts{
		DB:         dbName,
		Query:      query,
		ConfigPath: configPath,
		Dataset:    dataset,
		Limit:      limit,
		Timeout:    timeout,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

func handlePingDatabase(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dbName, err := req.RequireString("db")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	configPath := req.GetString("config", "")

	out, err := mcpcli.Ping(ctx, dbName, configPath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		text = fmt.Sprintf("ping ok: %s", dbName)
	}
	return mcp.NewToolResultText(text), nil
}

func handleListCollections(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dbName, err := req.RequireString("db")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	configPath := req.GetString("config", "")

	out, err := mcpcli.ListCollections(ctx, dbName, configPath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}
