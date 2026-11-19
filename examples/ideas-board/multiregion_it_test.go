package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/pgxadapter"
)

// tag::region-test[]

// TestRegional_WritesHomeRowsInBoundRegion proves region-scoped writes against a
// real multi-region cluster: the context-bound region lands each row in the right
// crdb_region of a REGIONAL BY ROW table.
//
// Gated on purpose. REGIONAL BY ROW needs a database with multiple regions, which a
// single Testcontainers node does not have. This test SKIPS unless
// CRDB_MULTIREGION=true; when enabled it reads an external cluster from CRDB_URL
// (expected to already have us-east1 and us-west1 added as regions). It never fails
// by default — it is skipped.
func TestRegional_WritesHomeRowsInBoundRegion(t *testing.T) {
	if os.Getenv("CRDB_MULTIREGION") != "true" {
		t.Skip("skipping: set CRDB_MULTIREGION=true and CRDB_URL to a multi-region cluster (regions us-east1, us-west1)")
	}
	url := os.Getenv("CRDB_URL")
	if url == "" {
		t.Skip("skipping: CRDB_MULTIREGION=true but CRDB_URL is unset")
	}
	ctx := context.Background()

	pool, err := pgxadapter.NewPool(ctx, url, []crdbgo.Option{})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	// The external multi-region database must already have the regions added:
	//   ALTER DATABASE <db> ADD REGION 'us-east1'; ADD REGION 'us-west1';
	if _, err := pool.RawPool().Exec(ctx, "DROP TABLE IF EXISTS regional_idea"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	for _, stmt := range strings.Split(RegionalByRowDDL, ";") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := pool.RawPool().Exec(ctx, stmt); err != nil {
			t.Fatalf("ddl %q: %v", stmt, err)
		}
	}

	svc := NewRegionalIdeaService(pool.RawPool())
	if err := svc.AddInUsEast(ctx, "idea-east", "Home me in the east"); err != nil {
		t.Fatalf("AddInUsEast: %v", err)
	}
	if err := svc.AddInUsWest(ctx, "idea-west", "Home me in the west"); err != nil {
		t.Fatalf("AddInUsWest: %v", err)
	}

	if got, err := svc.RegionOf(ctx, "idea-east"); err != nil || got != "us-east1" {
		t.Fatalf("RegionOf(idea-east) = %q, %v; want us-east1", got, err)
	}
	if got, err := svc.RegionOf(ctx, "idea-west"); err != nil || got != "us-west1" {
		t.Fatalf("RegionOf(idea-west) = %q, %v; want us-west1", got, err)
	}
}

// end::region-test[]
