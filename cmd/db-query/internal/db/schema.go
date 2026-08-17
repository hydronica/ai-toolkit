package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

var catalogColumns = []string{"object", "kind", "column", "data_type"}

const (
	sqliteCatalogQuery = `
SELECT m.name, m.type, p.name, p.type
FROM sqlite_master AS m
LEFT JOIN pragma_table_info(m.name) AS p
WHERE m.type IN ('table', 'view')
  AND m.name NOT LIKE 'sqlite_%'
ORDER BY m.name, p.cid`

	mysqlCatalogQuery = `
SELECT c.table_name,
       CASE t.table_type
         WHEN 'BASE TABLE' THEN 'table'
         WHEN 'VIEW' THEN 'view'
         ELSE LOWER(t.table_type)
       END,
       c.column_name,
       c.data_type
FROM information_schema.columns AS c
JOIN information_schema.tables AS t
  ON t.table_schema = c.table_schema AND t.table_name = c.table_name
WHERE c.table_schema = DATABASE()
  AND t.table_type IN ('BASE TABLE', 'VIEW')
ORDER BY c.table_name, c.ordinal_position`

	postgresCatalogQuery = `
SELECT CASE WHEN c.table_schema = 'public' THEN c.table_name
            ELSE c.table_schema || '.' || c.table_name
       END,
       CASE t.table_type
         WHEN 'BASE TABLE' THEN 'table'
         WHEN 'VIEW' THEN 'view'
         ELSE LOWER(t.table_type)
       END,
       c.column_name,
       c.data_type
FROM information_schema.columns AS c
JOIN information_schema.tables AS t
  ON t.table_schema = c.table_schema AND t.table_name = c.table_name
WHERE c.table_schema NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
  AND c.table_schema NOT LIKE 'pg_temp%'
  AND t.table_type IN ('BASE TABLE', 'VIEW')
ORDER BY c.table_schema, c.table_name, c.ordinal_position`

	oracleCatalogQuery = `
SELECT utc.table_name,
       CASE WHEN uv.view_name IS NOT NULL THEN 'view' ELSE 'table' END,
       utc.column_name,
       utc.data_type
FROM user_tab_columns utc
LEFT JOIN user_views uv ON uv.view_name = utc.table_name
ORDER BY utc.table_name, utc.column_id`
)

type catalogBuilder struct {
	limit     int
	objects   int
	last      string
	truncated bool
	out       QueryOutput
}

func newCatalogBuilder(limit int) *catalogBuilder {
	return &catalogBuilder{
		limit: limit,
		out: QueryOutput{
			Columns: catalogColumns,
			Rows:    make([]map[string]interface{}, 0),
		},
	}
}

func (b *catalogBuilder) allowObject() bool {
	if b.limit > 0 && b.objects >= b.limit {
		b.truncated = true
		return false
	}
	return true
}

func (b *catalogBuilder) add(object, kind, column, dataType string) bool {
	if object != b.last {
		if !b.allowObject() {
			return false
		}
		b.objects++
		b.last = object
	}
	b.out.Rows = append(b.out.Rows, map[string]interface{}{
		"object":    object,
		"kind":      kind,
		"column":    column,
		"data_type": dataType,
	})
	b.out.RowCount++
	return true
}

func (b *catalogBuilder) result() QueryOutput {
	b.out.Truncated = b.truncated
	return b.out
}

func (r *sqlRunner) ListSchema(ctx context.Context, limit int) (QueryOutput, error) {
	query, err := catalogQueryForDriver(r.DriverName())
	if err != nil {
		return QueryOutput{}, err
	}

	rows, err := r.QueryContext(ctx, query)
	if err != nil {
		return QueryOutput{}, fmt.Errorf("list schema: %w", err)
	}
	defer rows.Close()

	b := newCatalogBuilder(limit)
	for rows.Next() {
		var object, kind string
		var column, dataType sql.NullString
		if err := rows.Scan(&object, &kind, &column, &dataType); err != nil {
			return QueryOutput{}, fmt.Errorf("list schema: %w", err)
		}
		if !b.add(object, sqlTableKind(kind), column.String, dataType.String) {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return QueryOutput{}, fmt.Errorf("list schema: %w", err)
	}
	return b.result(), nil
}

func catalogQueryForDriver(driver string) (string, error) {
	switch driver {
	case "sqlite", "sqlite3":
		return sqliteCatalogQuery, nil
	case "mysql":
		return mysqlCatalogQuery, nil
	case "postgres", "pgx":
		return postgresCatalogQuery, nil
	case "oracle":
		return oracleCatalogQuery, nil
	default:
		return "", fmt.Errorf("schema listing is not supported for driver %q", driver)
	}
}

func sqlTableKind(tableType string) string {
	switch strings.ToUpper(strings.TrimSpace(tableType)) {
	case "BASE TABLE", "TABLE":
		return "table"
	case "VIEW":
		return "view"
	case "MATERIALIZED VIEW", "MATERIALIZED_VIEW":
		return "materialized_view"
	default:
		return strings.ToLower(tableType)
	}
}
