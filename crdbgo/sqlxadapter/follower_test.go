package sqlxadapter

import (
	"context"
	"testing"
	"time"
)

// A negative staleness must fail fast before the query is issued, so these run
// against a zero-value DB (nil underlying db) without a live cluster.

func TestFollowerReadAs_NegativeStalenessFailsFast(t *testing.T) {
	d := &DB{}
	ctx := context.Background()
	neg := -5 * time.Second

	if rows, err := d.QueryFollowerReadAs(ctx, neg, "SELECT 1"); err == nil || rows != nil {
		t.Errorf("QueryFollowerReadAs: expected error and nil rows, got rows=%v err=%v", rows, err)
	}
	if row, err := d.QueryRowFollowerReadAs(ctx, neg, "SELECT 1"); err == nil || row != nil {
		t.Errorf("QueryRowFollowerReadAs: expected error and nil row, got row=%v err=%v", row, err)
	}
	var dest int
	if err := d.GetFollowerReadAs(ctx, neg, &dest, "SELECT 1"); err == nil {
		t.Error("GetFollowerReadAs: expected error for negative staleness, got nil")
	}
	var list []int
	if err := d.SelectFollowerReadAs(ctx, neg, &list, "SELECT 1"); err == nil {
		t.Error("SelectFollowerReadAs: expected error for negative staleness, got nil")
	}
}
