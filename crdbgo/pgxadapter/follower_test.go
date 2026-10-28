package pgxadapter

import (
	"context"
	"testing"
	"time"
)

// A negative staleness must fail fast before the query is issued, so these run
// against a zero-value Pool (nil underlying pool) without a live cluster.

func TestQueryFollowerReadAs_NegativeStalenessFailsFast(t *testing.T) {
	p := &Pool{}
	rows, err := p.QueryFollowerReadAs(context.Background(), -5*time.Second, "SELECT 1")
	if err == nil {
		t.Fatal("expected error for negative staleness, got nil")
	}
	if rows != nil {
		t.Errorf("expected nil rows on error, got %v", rows)
	}
}

func TestQueryRowFollowerReadAs_NegativeStalenessFailsFast(t *testing.T) {
	p := &Pool{}
	row, err := p.QueryRowFollowerReadAs(context.Background(), -1*time.Second, "SELECT 1")
	if err == nil {
		t.Fatal("expected error for negative staleness, got nil")
	}
	if row != nil {
		t.Errorf("expected nil row on error, got %v", row)
	}
}
