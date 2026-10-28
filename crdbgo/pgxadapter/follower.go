package pgxadapter

import (
	"context"
	"time"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
	"github.com/jackc/pgx/v5"
)

// QueryFollowerRead executes query as a follower read by appending
// AS OF SYSTEM TIME follower_read_timestamp(). This routes the read to the
// nearest replica, reducing latency in multi-region deployments. Metrics are
// recorded if a [crdbgo.MetricsRecorder] is attached.
func (p *Pool) QueryFollowerRead(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	start := time.Now()
	fullQuery := query + " " + crdbgo.FollowerReadTimestamp()
	rows, err := p.pool.Query(ctx, fullQuery, args...)
	if err == nil && p.metrics != nil {
		p.metrics.RecordFollowerRead(time.Since(start))
	}
	return rows, err
}

// QueryRowFollowerRead executes query as a follower read and returns a single
// row. It appends AS OF SYSTEM TIME follower_read_timestamp() to the query.
func (p *Pool) QueryRowFollowerRead(ctx context.Context, query string, args ...any) pgx.Row {
	fullQuery := query + " " + crdbgo.FollowerReadTimestamp()
	return p.pool.QueryRow(ctx, fullQuery, args...)
}

// QueryFollowerReadAs is like [Pool.QueryFollowerRead] but reads at a caller-
// supplied staleness. A zero staleness uses follower_read_timestamp() (the
// default); a negative staleness returns an error without issuing the query, so
// an invalid value fails fast instead of silently reading current data.
func (p *Pool) QueryFollowerReadAs(ctx context.Context, staleness time.Duration, query string, args ...any) (pgx.Rows, error) {
	clause, err := crdbgo.FollowerReadClause(staleness)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	fullQuery := query + " " + clause
	rows, err := p.pool.Query(ctx, fullQuery, args...)
	if err == nil && p.metrics != nil {
		p.metrics.RecordFollowerRead(time.Since(start))
	}
	return rows, err
}

// QueryRowFollowerReadAs is like [Pool.QueryRowFollowerRead] but reads at a
// caller-supplied staleness. Unlike QueryRowFollowerRead it returns an error so
// an invalid staleness fails fast at call time rather than deferring to Scan; a
// zero staleness uses follower_read_timestamp().
func (p *Pool) QueryRowFollowerReadAs(ctx context.Context, staleness time.Duration, query string, args ...any) (pgx.Row, error) {
	clause, err := crdbgo.FollowerReadClause(staleness)
	if err != nil {
		return nil, err
	}
	fullQuery := query + " " + clause
	return p.pool.QueryRow(ctx, fullQuery, args...), nil
}
