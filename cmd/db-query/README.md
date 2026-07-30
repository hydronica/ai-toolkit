# db-query

A CLI for running read-only queries against configured databases and returning results as JSON, CSV, or a terminal table.

Supported backends:

- Oracle
- Postgres
- BigQuery
- MongoDB

## Install

From this directory:

```bash
go install .
```

Or build a local binary:

```bash
go build -o db-query .
```

## Configuration

Copy the example config and fill in your connection details:

```bash
cp config.example.toml config.toml
```

`db-query` loads `config.toml` by default. Override the path with `-config` or the `DB_QUERY_CONFIG` environment variable.

Each database is a `[[databases]]` entry with a unique `name` and `type`:

```toml
[[databases]]
name = "maindb"
type = "oracle"
connection = "oracle://user:password@host:1521/SERVICE"

[[databases]]
name = "iap"
type = "postgres"
host = "postgres.example.com"
db = "iap"
username = "srv-iap"
password = "secret"
sslmode = "require"
sslcert = "./certs/postgresql.crt"
sslkey = "./certs/postgresql.key"
sslrootcert = "./certs/root.crt"

[[databases]]
name = "warehouse"
type = "bigquery"
project = "my-gcp-project"
dataset = "analytics"
auth = "adc"

# Or with a service account key file (signal-api style):
# auth = "service_account"
# credentials = "/path/to/service-account.json"

[[databases]]
name = "reporting"
type = "mongo"
uri = "mongodb://user:password@host:27017/reporting_dev?authSource=admin"
db_name = "reporting_dev"
```

Postgres and MongoDB field names mirror [go-rapi](https://github.com/go-rapi) config conventions.

### Backend-specific notes

| Type | Required fields | Notes |
|------|-----------------|-------|
| `oracle` | `connection` | Full Oracle connection string |
| `postgres` | `host`, `db`, `username` | Optional SSL client certs; see [Postgres / Cloud SQL SSL](#postgres--cloud-sql-ssl) |
| `mysql` | `host`, `db`, `username` | Port defaults to `3306` when omitted from `host` |
| `sqlite` | `db` | File path (opened read-only); or set `connection` for a full SQLite URI |

### Postgres / Cloud SQL SSL

Cloud SQL over a private DNS name (e.g. `postgres-iap-preprod.sqlprod.private.linksynergy.com`) presents a server certificate whose CN is the **instance name** (`rm-gcp-iap-core-prod:postgres-iap-preprod`), not the DNS hostname. With `sslrootcert` set, the standard driver verifies hostname and fails.

Use `ssl_skip_hostname_verify = true` to verify the CA (when `sslrootcert` is set) without hostname matching:

```toml
[[databases]]
name = "iap"
type = "postgres"
host = "postgres-iap-preprod.sqlprod.private.linksynergy.com"
db = "iap"
username = "srv-iap"
password = "..."
sslmode = "require"
sslcert = "./postgresql.crt"
sslkey = "./postgresql.key"
sslrootcert = "./root.crt"
ssl_skip_hostname_verify = true
```

**Important:** client cert, key, and root CA must be issued for **that Cloud SQL instance**. Certs copied from another environment (e.g. signal-api deploy for a different instance) will fail.

Verify your certs match the instance:

```bash
# What the server presents (note the CA dnQualifier):
echo | openssl s_client -connect HOST:5432 -starttls postgres 2>/dev/null \
  | openssl x509 -noout -issuer

# What your root.crt is (must be the Server CA for this instance, not another):
openssl x509 -in root.crt -noout -subject -issuer

# What your client cert is (must be issued by this instance's Client CA):
openssl x509 -in postgresql.crt -noout -subject -issuer
```

For `postgres-iap-preprod`, download fresh **Server CA** and **Client certificate** from GCP Console → Cloud SQL → `postgres-iap-preprod` → Connections → Security. Each instance has unique CA dnQualifiers — mixing preprod/prod or `app` vs `postgres-iap-preprod` certs will not work.

`ssl_skip_hostname_verify` requires `sslrootcert` so the server certificate is verified against your CA without hostname matching.
| `bigquery` | `project` (or `bq-project`) | `auth`: `adc` or `service_account`. Dataset lives in your SQL (see below); config `dataset` / `-dataset` are optional shortcuts only |
| `mongo` | `uri`, `db_name` | `type = "mongodb"` is also accepted |

### BigQuery authentication

BigQuery supports two auth modes:

| `auth` | When to use | Setup |
|--------|-------------|-------|
| `adc` | Local development with your user account | `gcloud auth application-default login` |
| `service_account` | CI/production (same as signal-api) | Set `credentials` or `bq-auth` to a service account JSON path |

If `auth` is omitted, db-query picks automatically: `service_account` when `credentials` / `bq-auth` is set, otherwise `adc`.

```toml
[[databases]]
name = "warehouse"
type = "bigquery"
project = "rm-gcp-reporting-preprod"
auth = "adc"
```

Aliases for `adc`: `gcloud`, `application_default`. Aliases for `service_account`: `credentials`, `credentials_file`.

### BigQuery queries (console-style)

In the BigQuery console, the dataset is part of the table reference in your SQL — db-query works the same way. Pass the query as-is with backtick-qualified names; no `dataset` in config required:

```bash
db-query -db warehouse -query \
  'SELECT * FROM `rm-gcp-reporting-preprod.dwh_audit.advertiser_homepage_freshness` LIMIT 1'
```

Config `project` is used for the client connection and billing context. The project, dataset, and table in your SQL can reference any dataset you have access to.

**Optional:** `dataset` in config or `-dataset` on the CLI only helps when you use *short* unqualified table names (e.g. `SELECT * FROM events` instead of `` `project.dataset.events` ``). Most console-style queries don't need this.

**Location:** omit `location` from config for fully qualified queries — BigQuery infers it from the dataset. Only set `location` (e.g. `us-central1`) when you know the dataset's region. A wrong value like `US` causes `Dataset ... was not found in location US` errors when the dataset lives in a regional location.

## Usage

```bash
# List configured databases
db-query -list-dbs

# Test a connection
db-query -db iap -ping

# MongoDB — list collections in the configured database
db-query -db mongo-preprod -list-collections -format json

# Run a SQL query (Oracle, Postgres, BigQuery)
db-query -db maindb -query "SELECT 1 FROM dual"

# Pipe a query from stdin
echo "SELECT id, name FROM users LIMIT 10" | db-query -db iap

# BigQuery — same SQL you use in the console (dataset is in the query)
db-query -db warehouse -query \
  'SELECT * FROM `rm-gcp-reporting-preprod.dwh_audit.advertiser_homepage_freshness` LIMIT 1' \
  -format table
```

If only one database is configured, `-db` is optional.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-config` | `config.toml` | Path to TOML config file |
| `-db` | | Database name from config |
| `-query` | | Query text (or pipe via stdin) |
| `-list-dbs` | `false` | List configured database names and exit |
| `-list-collections` | `false` | MongoDB only: list collections and views and exit |
| `-ping` | `false` | Test connection and exit |
| `-dataset` | | BigQuery only: default dataset for *unqualified* table names (optional; prefer fully qualified SQL) |
| `-format` | `json` | Output format: `json`, `csv`, or `table` |
| `-limit` | `1000` | Maximum rows to return (`0` = unlimited) |
| `-timeout` | `5m` | Query timeout |

## Query formats

### SQL (Oracle, Postgres, BigQuery)

Pass a single read-only `SELECT` or `WITH` statement via `-query` or stdin.

Blocked for safety:

- Non-`SELECT` / non-`WITH` statements
- Write keywords (`INSERT`, `UPDATE`, `DELETE`, `DROP`, etc.)
- Multiple statements in one query

```bash
db-query -db iap -query "WITH active AS (SELECT id FROM users WHERE active) SELECT * FROM active"
```

### MongoDB

MongoDB uses JSON instead of SQL.

**List collections** — discover collection and view names (uses the MongoDB `listCollections` command):

```bash
db-query -db reporting -list-collections -format json
```

Example output:

```json
{
  "columns": ["name", "type"],
  "rows": [
    {"name": "Report", "type": "collection"},
    {"name": "ScheduledReport", "type": "collection"}
  ],
  "row_count": 2
}
```

**Find** — filter, projection, sort:

```bash
db-query -db reporting -query '{
  "collection": "ScheduledReport",
  "filter": {"user.entity_type": "publisher"},
  "projection": {"_id": 1, "emails": 1},
  "sort": {"_id": -1},
  "limit": 10
}'
```

**Aggregate** — pipeline:

```bash
db-query -db reporting -query '{
  "collection": "BatchmanJobs",
  "pipeline": [
    {"$match": {"name": "scheduled-report"}},
    {"$project": {"status": 1, "dates": 1}}
  ]
}'
```

Write stages such as `$out` and `$merge` are rejected.

## Output

All formats share the same underlying result shape. JSON output looks like:

```json
{
  "columns": ["id", "name"],
  "rows": [
    {"id": 1, "name": "alice"},
    {"id": 2, "name": "bob"}
  ],
  "row_count": 2,
  "truncated": false
}
```

When `-limit` stops early, `truncated` is `true` and table output includes a footer noting how many rows were shown.

## Examples

```bash
# JSON to stdout (default)
db-query -db maindb -query "SELECT SYSDATE FROM dual"

# Human-readable table
db-query -db iap -query "SELECT id, program_name FROM clients LIMIT 5"

# CSV for scripting
db-query -db warehouse -query "SELECT event_date, count(*) AS n FROM analytics.events GROUP BY 1" -format csv > events.csv

# Mongo find with table output
db-query -db reporting -query '{"collection":"Report","filter":{},"limit":5}'
```

## Development

With [just](https://github.com/casey/just) installed:

```bash
just          # run tests, then build bin/db-query
just test     # go test ./...
just build    # build bin/db-query
just clean    # remove bin/
```

Or manually:

```bash
go test ./...
go run . -config config.example.toml -list-dbs
./bin/db-query -list-dbs
```

## Using with AI agents

Agents invoke `db-query` as a **read-only shell tool** — no separate server required. JSON stdout is designed for machine parsing.

### 1. Install the skill (Cursor)

This repo includes a project skill at `cmd/db-query/.cursor/skills/db-query/SKILL.md`. Cursor agents auto-discover it when working in `cmd/db-query/`. It documents databases, query formats, and the agent workflow.

### 2. Build and configure

```bash
cd cmd/db-query
just build
cp config.example.toml config.toml   # edit with your connections
export DB_QUERY_CONFIG="$PWD/config.toml"   # optional; agents can pass -config
```

### 3. Agent discovery command

```bash
./bin/db-query -list-dbs
```

Example output:

```json
[
  {"name": "iap", "type": "postgres"},
  {"name": "warehouse", "type": "bigquery"},
  {"name": "maindb", "type": "oracle"},
  {"name": "mongo-preprod", "type": "mongo"}
]
```

### 4. Agent query pattern

```bash
./bin/db-query -db iap -query 'SELECT id FROM iap.clients LIMIT 5' -limit 100
```

- **stdout**: JSON `{ columns, rows, row_count, truncated }`
- **stderr**: errors and warnings
- **exit 0**: success; **exit 1**: failure

### 5. Optional: global install

```bash
go install .
# ensure $(go env GOPATH)/bin is on PATH
db-query -config /path/to/config.toml -db warehouse -query 'SELECT 1'
```

### MCP server (optional)

To expose db-query as native MCP tools in Cursor (instead of shell invocation), use the included stdio server. It wraps the same CLI — config, auth, and read-only enforcement stay unchanged.

#### Build

```bash
just build   # produces bin/db-query and bin/db-query-mcp
```

#### Cursor MCP config

Add to your MCP settings (Cursor Settings → MCP, or `.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "db-query": {
      "command": "/absolute/path/to/db-query/bin/db-query-mcp",
      "env": {
        "DB_QUERY_BIN": "/absolute/path/to/db-query/bin/db-query",
        "DB_QUERY_CONFIG": "/absolute/path/to/db-query/config.toml"
      }
    }
  }
}
```

For development without installing binaries:

```json
{
  "mcpServers": {
    "db-query": {
      "command": "go",
      "args": ["run", "./cmd/db-query-mcp"],
      "cwd": "/absolute/path/to/ai-toolkit/cmd/db-query",
      "env": {
        "DB_QUERY_BIN": "/absolute/path/to/db-query/bin/db-query",
        "DB_QUERY_CONFIG": "/absolute/path/to/db-query/config.toml"
      }
    }
  }
}
```

#### Tools exposed

| Tool | Purpose |
|------|---------|
| `list_databases` | Returns configured databases (`name`, `type`) as JSON |
| `list_collections` | MongoDB only: lists collections/views (`name`, `type`) |
| `run_query` | Runs a read-only query; args: `db`, `query`, optional `limit`, `dataset`, `config`, `timeout_seconds` |
| `ping_database` | Tests connectivity for a `db` name |

#### How it works

```mermaid
sequenceDiagram
    participant Cursor
    participant MCP as db-query-mcp
    participant CLI as db-query

    Cursor->>MCP: tools/call run_query
    MCP->>CLI: exec -db iap -query ... -format json
    CLI-->>MCP: JSON stdout
    MCP-->>Cursor: tool result text
```

The MCP process speaks JSON-RPC over **stdio** (stdin/stdout). It must not log to stdout — only the CLI wrapper writes there. Set `DB_QUERY_BIN` if the binary is not on `PATH` or beside `db-query-mcp` in `bin/`.

Shell + skill is still fine for agents that already have terminal access; MCP is useful when you want typed tools without granting arbitrary shell.
