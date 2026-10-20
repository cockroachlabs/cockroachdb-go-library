package vcluster_test

import (
	"context"
	"testing"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo/continuum/internal/ctest"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/continuum/vcluster"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/pgxadapter"
	"github.com/jackc/pgx/v5"
)

// readTags reads the session tags the vCluster hook should have applied, from
// inside the current transaction.
func readTags(ctx context.Context, tx pgx.Tx) (app, qos string, err error) {
	if err = tx.QueryRow(ctx, "SHOW application_name").Scan(&app); err != nil {
		return
	}
	err = tx.QueryRow(ctx, "SHOW default_transaction_quality_of_service").Scan(&qos)
	return
}

func TestExecuteTx_AppliesVClusterTags(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxadapter.NewPool(ctx, ctest.ConnString(t), nil)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	cases := []struct {
		name     string
		priority vcluster.Priority
		wantApp  string
		wantQoS  string
	}{
		{"analytics", vcluster.Low, "analytics", "background"},
		{"oltp", vcluster.High, "oltp", "critical"},
		{"reporting", vcluster.Normal, "reporting", "regular"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			txCtx := vcluster.With(ctx, c.name, c.priority)
			var app, qos string
			err := pool.ExecuteTx(txCtx, func(tx pgx.Tx) error {
				var e error
				app, qos, e = readTags(txCtx, tx)
				return e
			})
			if err != nil {
				t.Fatalf("ExecuteTx: %v", err)
			}
			if app != c.wantApp {
				t.Errorf("application_name = %q, want %q", app, c.wantApp)
			}
			if qos != c.wantQoS {
				t.Errorf("default_transaction_quality_of_service = %q, want %q", qos, c.wantQoS)
			}
		})
	}
}

// Without a vCluster context, ExecuteTx must not alter session tags.
func TestExecuteTx_NoVClusterContext(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxadapter.NewPool(ctx, ctest.ConnString(t), nil)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	var qos string
	err = pool.ExecuteTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SHOW default_transaction_quality_of_service").Scan(&qos)
	})
	if err != nil {
		t.Fatalf("ExecuteTx: %v", err)
	}
	if qos != "regular" {
		t.Errorf("qos = %q, want session default 'regular' when no vCluster tag set", qos)
	}
}
