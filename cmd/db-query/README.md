# db-query

Read-only CLI for querying configured databases. Returns JSON, CSV, or a terminal table.

Supported: Oracle, Postgres, MySQL, SQLite, BigQuery, MongoDB.

## Install

```bash
make db-query          # from repo root → scripts/db-query
go install .           # from this directory
```

## Configuration

```bash
cp config.example.toml config.toml
```

Loads `config.toml` from the binary's directory by default.

Each `[[databases]]` entry needs a unique `name` and `type`. If only one database is configured, `-db` is optional.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-db` | | Database name from config |
| `-query` | | Query text (or pipe via stdin) |
| `-format` | `json` | `json`, `csv`, or `table` |
| `-limit` | `1000` | Max rows (`0` = unlimited) |
| `-timeout` | `5m` | Query timeout |
| `-list-dbs` | | List configured databases and exit |
| `-list-collections` | | MongoDB: list collections/views |
| `-ping` | | Test connection and exit |
| `-dataset` | | BigQuery: default dataset for unqualified tables |
| `-mcp` | | Start MCP stdio server |

## Usage

### SQL (Oracle, Postgres, MySQL, SQLite, BigQuery)

Single read-only `SELECT` or `WITH` statement. Write keywords and multiple statements are blocked.

```bash
db-query -db maindb -query "SELECT id, name FROM users LIMIT 5"
db-query -db warehouse -query "SELECT count(*) FROM events" -format csv > out.csv
echo "SELECT 1" | db-query -db mydb
```

### MongoDB

Uses JSON queries with `collection`, `filter`, `projection`, `sort`, `limit`, or `pipeline`:

```bash
db-query -db reporting -list-collections

db-query -db reporting -query '{"collection":"Report","filter":{"active":true},"limit":10}'

db-query -db reporting -query '{"collection":"Jobs","pipeline":[{"$match":{"status":"done"}}]}'
```

Write stages (`$out`, `$merge`) are rejected.

## Output

```json
{"columns":["id","name"],"rows":[{"id":1,"name":"alice"}],"row_count":1,"truncated":false}
```

stdout: JSON/CSV/table. stderr: errors. Exit 0 on success, 1 on failure.

## MCP Server

Pass `-mcp` to start a stdio JSON-RPC server exposing the same functionality as native MCP tools.

Add to `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "db-query": {
      "command": "/path/to/scripts/db-query",
      "args": ["-mcp"]
    }
  }
}
```

Place `config.toml` beside the binary. Tools: `list_databases`, `list_collections`, `run_query`, `ping_database`.

## Development

```bash
go test ./...
make db-query
```
