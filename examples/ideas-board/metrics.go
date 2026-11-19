package main

import (
	"context"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/metrics"
	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/pgxadapter"
)

// tag::metrics-config[]

// newMeteredService builds a Service whose pool reports metrics into an
// InMemoryRecorder. The recorder is attached with pgxadapter.WithMetrics, after
// which every ExecuteTx call the library runs records a transaction, and each
// 40001 retry it absorbs records a retry — automatically, with no extra code in
// the upvote path. The returned recorder's Snapshot() exposes the running totals.
func newMeteredService(ctx context.Context, connString string) (*Service, *metrics.InMemoryRecorder, error) {
	rec := metrics.NewInMemoryRecorder()
	pool, err := pgxadapter.NewPool(ctx, connString,
		[]crdbgo.Option{crdbgo.WithMaxRetries(100)},
		pgxadapter.WithMetrics(rec),
	)
	if err != nil {
		return nil, nil, err
	}
	return NewService(pool), rec, nil
}

// end::metrics-config[]
