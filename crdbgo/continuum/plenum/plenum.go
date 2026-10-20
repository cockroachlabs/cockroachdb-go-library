// Package plenum provides read-only Cockroach Plenum storage insights for
// CockroachDB.
//
// Cockroach Plenum is the usage-based storage layer. This package surfaces
// per-table storage signals — estimated row counts and, where the running
// version exposes it, approximate on-disk size — using only customer-safe SHOW
// surfaces. It never reads crdb_internal (which is restricted in production
// clusters, including v26.2).
//
// Every signal is version-tolerant: if a column or surface is unavailable on the
// running cluster, the corresponding field degrades to nil rather than failing
// the whole summary.
package plenum

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TableStorage is the Cockroach Plenum storage signal for a single table.
// Rows and Bytes are best-effort: a nil pointer means the running CockroachDB
// version did not expose that signal, not that the value is zero.
type TableStorage struct {
	// Schema is the schema the table belongs to (e.g. "public").
	Schema string
	// Table is the table name.
	Table string
	// Rows is the estimated row count, or nil when unavailable.
	Rows *int64
	// Bytes is the approximate on-disk size in bytes, or nil when unavailable.
	Bytes *int64
}

// QualifiedName returns the schema-qualified table name (e.g. "public.orders").
func (t TableStorage) QualifiedName() string {
	return t.Schema + "." + t.Table
}

// Summary returns a Cockroach Plenum storage summary for every table in the
// pool's current database. Row counts come from SHOW TABLES (estimated_row_count)
// and approximate sizes from SHOW RANGES ... WITH TABLES, DETAILS (range_size).
//
// The summary is defensive: a table always appears in the result even if one of
// its signals is unavailable on this cluster version, and a failure to read the
// size surface leaves Bytes nil rather than erroring. An error is returned only
// when the table list itself cannot be read.
func Summary(ctx context.Context, pool *pgxpool.Pool) ([]TableStorage, error) {
	db, err := currentDatabase(ctx, pool)
	if err != nil {
		return nil, err
	}

	sizes := tableBytes(ctx, pool, db) // best-effort; empty on any failure

	tables, err := tableRows(ctx, pool, db)
	if err != nil {
		return nil, err
	}

	out := make([]TableStorage, 0, len(tables))
	for _, t := range tables {
		ts := TableStorage{Schema: t.schema, Table: t.table, Rows: t.rows}
		if b, ok := sizes[key(t.schema, t.table)]; ok {
			b := b
			ts.Bytes = &b
		}
		out = append(out, ts)
	}
	return out, nil
}

type tableRow struct {
	schema string
	table  string
	rows   *int64
}

// tableRows reads the table list with estimated row counts. It falls back to a
// bare table list if the running version does not expose estimated_row_count or
// the type filter.
func tableRows(ctx context.Context, pool *pgxpool.Pool, db string) ([]tableRow, error) {
	q := "SELECT schema_name, table_name, estimated_row_count " +
		"FROM [SHOW TABLES FROM " + ident(db) + "] WHERE type = 'table'"
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return tableRowsFallback(ctx, pool, db)
	}
	defer rows.Close()

	var out []tableRow
	for rows.Next() {
		var schema, table string
		var rc *int64
		if err := rows.Scan(&schema, &table, &rc); err != nil {
			rows.Close()
			return tableRowsFallback(ctx, pool, db)
		}
		out = append(out, tableRow{schema: schema, table: table, rows: rc})
	}
	if err := rows.Err(); err != nil {
		return tableRowsFallback(ctx, pool, db)
	}
	return out, nil
}

// tableRowsFallback reads only the schema/table list, leaving row counts nil,
// for clusters that do not expose estimated_row_count.
func tableRowsFallback(ctx context.Context, pool *pgxpool.Pool, db string) ([]tableRow, error) {
	q := "SELECT schema_name, table_name FROM [SHOW TABLES FROM " + ident(db) + "]"
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []tableRow
	for rows.Next() {
		var schema, table string
		if err := rows.Scan(&schema, &table); err != nil {
			return nil, err
		}
		out = append(out, tableRow{schema: schema, table: table})
	}
	return out, rows.Err()
}

// tableBytes aggregates approximate on-disk size per table from SHOW RANGES.
// Any failure or missing column yields an empty map — sizes are never fatal.
func tableBytes(ctx context.Context, pool *pgxpool.Pool, db string) map[string]int64 {
	out := map[string]int64{}
	q := "SELECT schema_name, table_name, sum(range_size)::INT8 AS bytes " +
		"FROM [SHOW RANGES FROM DATABASE " + ident(db) + " WITH TABLES, DETAILS] " +
		"GROUP BY schema_name, table_name"
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var schema, table string
		var bytes *int64
		if err := rows.Scan(&schema, &table, &bytes); err != nil {
			return out
		}
		if bytes != nil {
			out[key(schema, table)] = *bytes
		}
	}
	return out
}

func currentDatabase(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var db string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&db); err != nil {
		return "", err
	}
	return db, nil
}

func key(schema, table string) string {
	return schema + "\x00" + table
}

// ident quotes a SQL identifier, escaping embedded double-quotes.
func ident(name string) string {
	quoted := make([]byte, 0, len(name)+2)
	quoted = append(quoted, '"')
	for i := 0; i < len(name); i++ {
		if name[i] == '"' {
			quoted = append(quoted, '"')
		}
		quoted = append(quoted, name[i])
	}
	quoted = append(quoted, '"')
	return string(quoted)
}
