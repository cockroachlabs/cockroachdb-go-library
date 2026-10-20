// Package ddl provides DDL builder helpers for CockroachDB-specific features
// such as inline index creation (~25x faster) and computed columns (STORED only).
package ddl

import (
	"fmt"
	"strings"
)

// ColumnDef represents a column definition for CreateTableWithIndexes.
type ColumnDef struct {
	Name       string
	Type       string
	PrimaryKey bool
	NotNull    bool
	Default    string
}

// IndexDef represents an inline index for CreateTableWithIndexes.
type IndexDef struct {
	Name    string
	Columns []string
	Unique  bool
	Storing []string
}

// ComputedColumnDef represents a computed column definition.
type ComputedColumnDef struct {
	Name       string
	Type       string
	Expression string
	NotNull    bool
}

// CreateTableWithIndexes generates a CREATE TABLE statement with indexes
// inlined for CockroachDB performance (~25x speedup vs separate CREATE INDEX).
func CreateTableWithIndexes(table string, columns []ColumnDef, indexes []IndexDef) string {
	var b strings.Builder
	fmt.Fprintf(&b, "CREATE TABLE %s (\n", table)

	var parts []string
	for _, col := range columns {
		def := fmt.Sprintf("  %s %s", col.Name, col.Type)
		if col.PrimaryKey {
			def += " PRIMARY KEY"
		}
		if col.NotNull {
			def += " NOT NULL"
		}
		if col.Default != "" {
			def += " DEFAULT " + col.Default
		}
		parts = append(parts, def)
	}

	for _, idx := range indexes {
		unique := ""
		if idx.Unique {
			unique = "UNIQUE "
		}
		def := fmt.Sprintf("  %sINDEX %s (%s)", unique, idx.Name, strings.Join(idx.Columns, ", "))
		if len(idx.Storing) > 0 {
			def += " STORING (" + strings.Join(idx.Storing, ", ") + ")"
		}
		parts = append(parts, def)
	}

	b.WriteString(strings.Join(parts, ",\n"))
	b.WriteString("\n)")
	return b.String()
}

// ComputedColumnDDL generates ALTER TABLE ADD COLUMN DDL for a computed column.
// CockroachDB requires computed columns to be STORED.
func ComputedColumnDDL(table string, col ComputedColumnDef) string {
	notNull := ""
	if col.NotNull {
		notNull = " NOT NULL"
	}
	colType := col.Type
	if colType == "" {
		colType = "INT8"
	}
	return fmt.Sprintf(
		"ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s AS (%s) STORED%s",
		table, col.Name, colType, col.Expression, notNull,
	)
}
