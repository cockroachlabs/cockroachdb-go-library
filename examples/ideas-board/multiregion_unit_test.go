package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeRow returns a fixed region string from Scan, for RegionOf.
type fakeRow struct{ region string }

func (r fakeRow) Scan(dest ...any) error {
	if len(dest) > 0 {
		if p, ok := dest[0].(*string); ok {
			*p = r.region
		}
	}
	return nil
}

// fakeQuerier records the last Exec args so the region-binding logic can be tested
// without a database.
type fakeQuerier struct {
	lastExecArgs []any
	row          fakeRow
}

func (q *fakeQuerier) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row { return q.row }
func (q *fakeQuerier) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	q.lastExecArgs = args
	return pgconn.CommandTag{}, nil
}

// TestAddInRegion_RequiresBoundRegion proves AddInRegion refuses to write when no
// region is bound into the context.
func TestAddInRegion_RequiresBoundRegion(t *testing.T) {
	svc := NewRegionalIdeaService(&fakeQuerier{})
	if err := svc.AddInRegion(context.Background(), "idea-1", "No region"); err == nil {
		t.Fatal("AddInRegion with no bound region: want error, got nil")
	}
}

// TestAddInUsEast_BindsRegionIntoWrite proves AddInUsEast binds us-east1 into the
// context and the service writes it as the crdb_region value.
func TestAddInUsEast_BindsRegionIntoWrite(t *testing.T) {
	q := &fakeQuerier{}
	svc := NewRegionalIdeaService(q)

	if err := svc.AddInUsEast(context.Background(), "idea-1", "Home me east"); err != nil {
		t.Fatalf("AddInUsEast: %v", err)
	}
	if len(q.lastExecArgs) != 3 || q.lastExecArgs[2] != "us-east1" {
		t.Fatalf("Exec region arg = %v, want us-east1", q.lastExecArgs)
	}
}

// TestRegionOf_ReadsRegionColumn proves RegionOf returns the crdb_region value.
func TestRegionOf_ReadsRegionColumn(t *testing.T) {
	svc := NewRegionalIdeaService(&fakeQuerier{row: fakeRow{region: "us-west1"}})
	got, err := svc.RegionOf(context.Background(), "idea-1")
	if err != nil {
		t.Fatalf("RegionOf: %v", err)
	}
	if got != "us-west1" {
		t.Fatalf("RegionOf = %q, want us-west1", got)
	}
}
