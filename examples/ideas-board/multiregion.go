package main

import (
	"context"
	"errors"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
)

// tag::region-config[]

// RegionalByRowDDL makes regional_idea a REGIONAL BY ROW table. On a multi-region
// database (one created with ALTER DATABASE ... ADD REGION ...) this adds the
// hidden crdb_region column the service writes, so each row homes in the region it
// names rather than wherever the gateway happens to be.
const RegionalByRowDDL = `CREATE TABLE IF NOT EXISTS regional_idea (
	id   STRING PRIMARY KEY,
	text STRING NOT NULL
);
ALTER TABLE regional_idea SET LOCALITY REGIONAL BY ROW;`

// end::region-config[]

// tag::inregion-usage[]

// RegionalIdeaService writes region-scoped rows to the REGIONAL BY ROW
// regional_idea table. Unlike the Java demo's @Region annotation, this library
// routes by context: AddInUsEast/AddInUsWest bind a crdbgo.RegionContext into the
// context, and AddInRegion reads it back and writes the bound region into the
// hidden crdb_region column.
type RegionalIdeaService struct {
	q querier
}

// NewRegionalIdeaService builds a region-aware writer over the given querier
// (pool.RawPool() satisfies it).
func NewRegionalIdeaService(q querier) *RegionalIdeaService {
	return &RegionalIdeaService{q: q}
}

// AddInUsEast stores an idea homed in us-east1.
func (s *RegionalIdeaService) AddInUsEast(ctx context.Context, id, text string) error {
	return s.AddInRegion(crdbgo.WithRegionContext(ctx, &crdbgo.RegionContext{Region: "us-east1"}), id, text)
}

// AddInUsWest stores an idea homed in us-west1.
func (s *RegionalIdeaService) AddInUsWest(ctx context.Context, id, text string) error {
	return s.AddInRegion(crdbgo.WithRegionContext(ctx, &crdbgo.RegionContext{Region: "us-west1"}), id, text)
}

// AddInRegion writes the row into the region bound into ctx via
// crdbgo.WithRegionContext. crdb_region is set explicitly so the REGIONAL BY ROW
// row homes where the context says, not where the gateway is. It errors when no
// region is bound.
func (s *RegionalIdeaService) AddInRegion(ctx context.Context, id, text string) error {
	rc, ok := crdbgo.RegionFromContext(ctx)
	if !ok || rc.Region == "" {
		return errors.New("no region bound; use AddInUsEast/AddInUsWest or WithRegionContext")
	}
	_, err := s.q.Exec(ctx,
		"INSERT INTO regional_idea (id, text, crdb_region) VALUES ($1, $2, $3)",
		id, text, rc.Region)
	return err
}

// end::inregion-usage[]

// RegionOf returns the crdb_region a row actually landed in, or "" if absent.
func (s *RegionalIdeaService) RegionOf(ctx context.Context, id string) (string, error) {
	var region string
	err := s.q.QueryRow(ctx,
		"SELECT crdb_region::STRING FROM regional_idea WHERE id = $1", id).Scan(&region)
	return region, err
}
