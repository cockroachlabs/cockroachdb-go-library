package crdbtest

import (
	"context"
	"database/sql"
	"testing"
)

// Harness manages test lifecycle for CockroachDB integration tests: connection
// setup, schema execution, and cleanup. Create one with [NewHarness].
type Harness struct {
	t       *testing.T
	connStr string
	ddl     []string
	setupFn func(connStr string) error
}

// HarnessOption configures a [Harness] during creation.
type HarnessOption func(*Harness)

// NewHarness creates a test harness that runs optional setup functions and DDL
// statements. It registers a t.Cleanup handler for teardown.
func NewHarness(t *testing.T, opts ...HarnessOption) *Harness {
	t.Helper()
	h := &Harness{t: t}
	for _, opt := range opts {
		opt(h)
	}

	if h.setupFn != nil && h.connStr != "" {
		if err := h.setupFn(h.connStr); err != nil {
			t.Fatalf("crdbtest: setup function failed: %v", err)
		}
	}

	if len(h.ddl) > 0 && h.connStr != "" {
		if err := ExecuteSchema(context.Background(), h.connStr, h.ddl); err != nil {
			t.Fatalf("crdbtest: schema execution failed: %v", err)
		}
	}

	t.Cleanup(func() {})

	return h
}

// WithConnectionString sets the CockroachDB connection string for the harness.
func WithConnectionString(connStr string) HarnessOption {
	return func(h *Harness) { h.connStr = connStr }
}

// WithSchema adds DDL statements to execute during harness setup (e.g.,
// CREATE TABLE, CREATE INDEX).
func WithSchema(ddl ...string) HarnessOption {
	return func(h *Harness) { h.ddl = append(h.ddl, ddl...) }
}

// WithSetupFunc registers a custom setup function that runs before schema
// execution. Use it for seed data, driver-specific pool setup, etc.
func WithSetupFunc(fn func(connStr string) error) HarnessOption {
	return func(h *Harness) { h.setupFn = fn }
}

// ConnectionString returns the CockroachDB connection string configured for
// this harness.
func (h *Harness) ConnectionString() string {
	return h.connStr
}

// ExecuteSchema opens a connection using database/sql and runs each DDL
// statement sequentially. Requires a registered pgx stdlib driver or
// any Postgres-compatible driver.
func ExecuteSchema(ctx context.Context, connStr string, ddl []string) error {
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return err
	}
	defer db.Close()

	for _, stmt := range ddl {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
