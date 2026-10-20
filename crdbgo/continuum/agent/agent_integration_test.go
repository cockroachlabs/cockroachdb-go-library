package agent_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo/continuum/agent"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/continuum/internal/ctest"
)

func TestOpsTools_HealthAndStorageSummary(t *testing.T) {
	ctx := context.Background()
	pool := ctest.Pool(t)
	table := ctest.UniqueTable("agent_ops")

	if _, err := pool.Exec(ctx, fmt.Sprintf(
		"CREATE TABLE %s (id INT PRIMARY KEY)", table)); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DROP TABLE IF EXISTS "+table) })

	reg := agent.OpsTools(pool)

	// Both built-in tools are listed and non-destructive.
	listed := map[string]bool{}
	for _, d := range reg.ListTools() {
		listed[d.Name] = true
	}
	if !listed["health"] || !listed["storage_summary"] {
		t.Fatalf("expected health + storage_summary tools, got %v", listed)
	}

	// health → status ok against the live node.
	got, err := reg.Invoke("health", nil)
	if err != nil {
		t.Fatalf("Invoke health: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok || m["status"] != "ok" {
		t.Fatalf("health result = %v, want status ok", got)
	}

	// storage_summary → includes the created table.
	res, err := reg.Invoke("storage_summary", nil)
	if err != nil {
		t.Fatalf("Invoke storage_summary: %v", err)
	}
	rows, ok := res.([]map[string]any)
	if !ok {
		t.Fatalf("storage_summary result type = %T, want []map[string]any", res)
	}
	var found bool
	for _, r := range rows {
		if r["table"] == table {
			found = true
			// rows/bytes keys always present (nil when unavailable).
			if _, has := r["rows"]; !has {
				t.Error("storage_summary row missing 'rows' key")
			}
			if _, has := r["bytes"]; !has {
				t.Error("storage_summary row missing 'bytes' key")
			}
		}
	}
	if !found {
		t.Errorf("storage_summary did not include %q", table)
	}
}
