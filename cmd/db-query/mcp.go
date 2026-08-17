package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"
	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/db"
)

const defaultMCPQueryLimit = 1000

func serveMCP(cfg *config.Config) {
	s := server.NewMCPServer(
		"db-query",
		version,
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	s.AddTool(mcpListDatabasesTool(), mcpHandleListDatabases(cfg))
	s.AddTool(mcpRunQueryTool(), mcpHandleRunQuery(cfg))
	s.AddTool(mcpPingDatabaseTool(), mcpHandlePingDatabase(cfg))
	s.AddTool(mcpListSchemaTool("list_schema"), mcpHandleListSchema(cfg))
	s.AddTool(mcpListSchemaTool("list_collections"), mcpHandleListSchema(cfg))

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "db-query mcp: %v\n", err)
		os.Exit(1)
	}
}

func mcpListDatabasesTool() mcp.Tool {
	return mcp.NewTool("list_databases",
		mcp.WithDescription("List databases configured in db-query (name and type). Call this before run_query to pick a valid -db value."),
	)
}

func mcpRunQueryTool() mcp.Tool {
	return mcp.NewTool("run_query",
		mcp.WithDescription("Run a read-only SQL or MongoDB query against a configured database. Returns JSON with columns, rows, row_count, and truncated."),
		mcp.WithString("db",
			mcp.Required(),
			mcp.Description("Database name from list_databases"),
		),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("SQL SELECT or MongoDB find/aggregate JSON"),
		),
		mcp.WithString("dataset",
			mcp.Description("BigQuery only: default dataset for unqualified table names"),
		),
		mcp.WithNumber("limit",
			mcp.Description(fmt.Sprintf("Maximum rows to return (default %d; 0 = unlimited)", defaultMCPQueryLimit)),
		),
		mcp.WithNumber("timeout_seconds",
			mcp.Description("Query timeout in seconds (default 300)"),
		),
	)
}

func mcpPingDatabaseTool() mcp.Tool {
	return mcp.NewTool("ping_database",
		mcp.WithDescription("Test connectivity to a configured database without running a query."),
		mcp.WithString("db",
			mcp.Required(),
			mcp.Description("Database name from list_databases"),
		),
	)
}

func mcpListSchemaTool(name string) mcp.Tool {
	desc := "List tables, views, or collections and their columns. Returns JSON with object, kind, column, and data_type. Limit applies to objects, not column rows."
	if name == "list_collections" {
		desc = "Deprecated alias of list_schema. " + desc
	}
	return mcp.NewTool(name,
		mcp.WithDescription(desc),
		mcp.WithString("db",
			mcp.Required(),
			mcp.Description("Database name from list_databases"),
		),
		mcp.WithString("dataset",
			mcp.Description("BigQuery only: dataset to list (required when the config has no default dataset)"),
		),
		mcp.WithNumber("limit",
			mcp.Description(fmt.Sprintf("Maximum objects to return (default %d; 0 = unlimited)", defaultMCPQueryLimit)),
		),
	)
}

func mcpHandleListDatabases(cfg *config.Config) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		out, err := json.Marshal(cfg.Summaries())
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(out)), nil
	}
}

func mcpHandleRunQuery(cfg *config.Config) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dbName, err := req.RequireString("db")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		query, err := req.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		dataset := req.GetString("dataset", "")
		limit := defaultMCPQueryLimit
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

		dbCfg, err := cfg.Find(dbName)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		runner, err := db.Connect(ctx, dbCfg)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		defer runner.Close(ctx)

		opts := db.QueryOptions{
			Limit:   limit,
			Dataset: strings.TrimSpace(dataset),
		}

		output, err := runner.RunQuery(ctx, query, opts)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		out, err := json.Marshal(output)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(out)), nil
	}
}

func mcpHandlePingDatabase(cfg *config.Config) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dbName, err := req.RequireString("db")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		dbCfg, err := cfg.Find(dbName)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		runner, err := db.Connect(ctx, dbCfg)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		defer runner.Close(ctx)

		if err := runner.Ping(ctx, db.QueryOptions{}); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("ping ok: %s (%s)", dbCfg.Name(), dbCfg.Type())), nil
	}
}

func mcpHandleListSchema(cfg *config.Config) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dbName, err := req.RequireString("db")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		dataset := req.GetString("dataset", "")
		limit := defaultMCPQueryLimit
		if raw, ok := req.GetArguments()["limit"]; ok && raw != nil {
			switch v := raw.(type) {
			case float64:
				limit = int(v)
			case int:
				limit = v
			}
		}

		dbCfg, err := cfg.Find(dbName)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()

		runner, err := db.Connect(ctx, dbCfg)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		defer runner.Close(ctx)

		output, err := runner.ListSchema(ctx, db.QueryOptions{
			Limit:   limit,
			Dataset: strings.TrimSpace(dataset),
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		out, err := json.Marshal(output)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(out)), nil
	}
}
