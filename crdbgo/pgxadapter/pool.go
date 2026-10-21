package pgxadapter

import (
	"context"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool wraps a [pgxpool.Pool] with CockroachDB-specific features: automatic
// transaction retries, follower reads, health checking, and metrics recording.
// Create one with [NewPool].
type Pool struct {
	pool    *pgxpool.Pool
	config  *crdbgo.Config
	metrics crdbgo.MetricsRecorder
}

// PoolOption configures a [Pool] after creation. These are pgxadapter-specific
// options, separate from [crdbgo.Option].
type PoolOption func(*Pool)

// WithMetrics attaches a [crdbgo.MetricsRecorder] to the pool for recording
// transaction durations, retry counts, and follower read latencies.
func WithMetrics(m crdbgo.MetricsRecorder) PoolOption {
	return func(p *Pool) { p.metrics = m }
}

// NewPool creates a CockroachDB-aware connection pool. It parses the connection
// string, creates the underlying pgxpool.Pool, and applies both core [crdbgo.Option]
// values and pgxadapter-specific [PoolOption] values.
func NewPool(ctx context.Context, connString string, opts []crdbgo.Option, poolOpts ...PoolOption) (*Pool, error) {
	cfg := crdbgo.NewConfig(opts...)

	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}

	raw, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, err
	}

	p := &Pool{
		pool:   raw,
		config: cfg,
	}
	for _, opt := range poolOpts {
		opt(p)
	}
	return p, nil
}

// Close releases all connections in the underlying pgxpool.Pool.
func (p *Pool) Close() {
	p.pool.Close()
}

// Ping verifies the connection to CockroachDB is alive.
func (p *Pool) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

// Check implements crdbgo.HealthChecker.
func (p *Pool) Check(ctx context.Context) error {
	return p.Ping(ctx)
}

// RawPool returns the underlying [pgxpool.Pool] for direct pgx operations
// that bypass the CockroachDB retry and metrics wrappers.
func (p *Pool) RawPool() *pgxpool.Pool {
	return p.pool
}

// compile-time interface check
var _ crdbgo.HealthChecker = (*Pool)(nil)
