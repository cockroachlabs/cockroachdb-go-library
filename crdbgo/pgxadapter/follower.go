package pgxadapter

import (
	"context"
	"time"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Follower reads apply AS OF SYSTEM TIME to the *transaction*, not by appending
// the clause to the query text. Appending (`<query> AS OF SYSTEM TIME ...`) is a
// syntax error for any query with a WHERE/ORDER BY/LIMIT clause, because in
// CockroachDB the AOST clause belongs right after the table in FROM. Running the
// read inside a read-only transaction that issues `SET TRANSACTION AS OF SYSTEM
// TIME ...` first works for any query shape.

// followerRows wraps the pgx.Rows of a follower read so that closing the rows
// also ends the owning transaction and returns the connection to the pool.
type followerRows struct {
	pgx.Rows
	tx   pgx.Tx
	conn *pgxpool.Conn
	ctx  context.Context
}

func (r *followerRows) Close() {
	r.Rows.Close()
	_ = r.tx.Rollback(r.ctx) // read-only: rollback just releases the snapshot
	r.conn.Release()
}

// followerRow wraps a single-row follower read so that scanning (which consumes
// the row) also ends the transaction and releases the connection.
type followerRow struct {
	row  pgx.Row
	done func()
}

func (r followerRow) Scan(dest ...any) error {
	err := r.row.Scan(dest...)
	r.done()
	return err
}

// errorRow lets the no-error QueryRow* helpers report an acquire/begin failure
// through Scan instead of panicking or silently returning nil.
type errorRow struct{ err error }

func (r errorRow) Scan(_ ...any) error { return r.err }

// queryFollowerRead runs query in a read-only transaction pinned to aostClause
// (e.g. "AS OF SYSTEM TIME follower_read_timestamp()") and returns rows that
// own the transaction until closed.
func (p *Pool) queryFollowerRead(ctx context.Context, aostClause, query string, args ...any) (pgx.Rows, error) {
	start := time.Now()
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		conn.Release()
		return nil, err
	}
	if _, err := tx.Exec(ctx, "SET TRANSACTION "+aostClause); err != nil {
		_ = tx.Rollback(ctx)
		conn.Release()
		return nil, err
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		_ = tx.Rollback(ctx)
		conn.Release()
		return nil, err
	}
	if p.metrics != nil {
		p.metrics.RecordFollowerRead(time.Since(start))
	}
	return &followerRows{Rows: rows, tx: tx, conn: conn, ctx: ctx}, nil
}

// queryRowFollowerRead is the single-row variant of queryFollowerRead.
func (p *Pool) queryRowFollowerRead(ctx context.Context, aostClause, query string, args ...any) pgx.Row {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return errorRow{err: err}
	}
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		conn.Release()
		return errorRow{err: err}
	}
	if _, err := tx.Exec(ctx, "SET TRANSACTION "+aostClause); err != nil {
		_ = tx.Rollback(ctx)
		conn.Release()
		return errorRow{err: err}
	}
	done := func() {
		_ = tx.Rollback(ctx)
		conn.Release()
	}
	return followerRow{row: tx.QueryRow(ctx, query, args...), done: done}
}

// QueryFollowerRead executes query as a follower read: it runs inside a
// read-only transaction pinned with AS OF SYSTEM TIME follower_read_timestamp(),
// routing the read to the nearest replica and reducing latency in multi-region
// deployments. Close the returned rows to release the connection. Metrics are
// recorded if a [crdbgo.MetricsRecorder] is attached.
func (p *Pool) QueryFollowerRead(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	return p.queryFollowerRead(ctx, crdbgo.FollowerReadTimestamp(), query, args...)
}

// QueryRowFollowerRead executes query as a follower read and returns a single
// row, pinned with AS OF SYSTEM TIME follower_read_timestamp(). The connection
// is released when the row is scanned.
func (p *Pool) QueryRowFollowerRead(ctx context.Context, query string, args ...any) pgx.Row {
	return p.queryRowFollowerRead(ctx, crdbgo.FollowerReadTimestamp(), query, args...)
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
	return p.queryFollowerRead(ctx, clause, query, args...)
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
	return p.queryRowFollowerRead(ctx, clause, query, args...), nil
}
