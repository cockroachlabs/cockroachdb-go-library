package pgxadapter

import (
	"context"
	"time"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo"
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
