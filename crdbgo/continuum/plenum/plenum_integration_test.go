package plenum_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo/continuum/internal/ctest"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/continuum/plenum"
)

func TestSummary_IncludesCreatedTable(t *testing.T) {
	ctx := context.Background()
	pool := ctest.Pool(t)
	table := ctest.UniqueTable("plenum")

	if _, err := pool.Exec(ctx, fmt.Sprintf(
		"CREATE TABLE %s (id INT PRIMARY KEY, name TEXT)", table)); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP TABLE IF EXISTS "+table)
	})
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		"INSERT INTO %s (id, name) VALUES (1,'a'), (2,'b')", table)); err != nil {
		t.Fatalf("insert: %v", err)
	}

	summary, err := plenum.Summary(ctx, pool)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}

	var found *plenum.TableStorage
	for i := range summary {
		if summary[i].Table == table {
			found = &summary[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("Summary did not include created table %q; got %d tables", table, len(summary))
	}
	if found.Schema == "" {
		t.Error("expected a non-empty schema")
	}
	// Rows and Bytes are best-effort: optional, but non-negative when present.
	if found.Rows != nil && *found.Rows < 0 {
		t.Errorf("rows = %d, want >= 0", *found.Rows)
	}
	if found.Bytes != nil && *found.Bytes < 0 {
		t.Errorf("bytes = %d, want >= 0", *found.Bytes)
	}
	if got := found.QualifiedName(); got != found.Schema+"."+found.Table {
		t.Errorf("QualifiedName = %q, unexpected", got)
	}
}
