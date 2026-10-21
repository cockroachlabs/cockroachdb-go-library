package sqlxadapter

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
	"github.com/jmoiron/sqlx"

	// Register pgx as the database/sql driver for CockroachDB.
	_ "github.com/jackc/pgx/v5/stdlib"
)

// DB wraps a sqlx.DB with CockroachDB-specific features: transaction retry,
// follower reads, health checking, and metrics.
type DB struct {
	db      *sqlx.DB
	config  *crdbgo.Config
	metrics crdbgo.MetricsRecorder
}

// DBOption configures a [DB] after creation. These are sqlxadapter-specific
// options, separate from [crdbgo.Option].
type DBOption func(*DB)

// WithMetrics attaches a [crdbgo.MetricsRecorder] to the DB for recording
// transaction durations, retry counts, and follower read latencies.
func WithMetrics(m crdbgo.MetricsRecorder) DBOption {
	return func(d *DB) { d.metrics = m }
}

// Open creates a new DB by connecting to the given DSN using pgx/stdlib.
// The driverName is always "pgx" (registered by the pgx/v5/stdlib import).
func Open(dsn string, opts []crdbgo.Option, dbOpts ...DBOption) (*DB, error) {
	raw, err := sqlx.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	return newDB(raw, opts, dbOpts...), nil
}

// NewFromDB creates a DB from an existing sqlx.DB. Use this when you already
// have a configured sqlx.DB and want to add CockroachDB features.
func NewFromDB(raw *sqlx.DB, opts []crdbgo.Option, dbOpts ...DBOption) *DB {
	return newDB(raw, opts, dbOpts...)
}

func newDB(raw *sqlx.DB, opts []crdbgo.Option, dbOpts ...DBOption) *DB {
	cfg := crdbgo.NewConfig(opts...)
	d := &DB{
		db:     raw,
		config: cfg,
	}
	for _, opt := range dbOpts {
		opt(d)
	}
	return d
}

// Close closes the underlying sqlx.DB.
func (d *DB) Close() error {
	return d.db.Close()
}

// Ping verifies the database connection.
func (d *DB) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

// Check implements crdbgo.HealthChecker.
func (d *DB) Check(ctx context.Context) error {
	return d.Ping(ctx)
}

// RawDB returns the underlying [sqlx.DB] for direct access to sqlx operations
// that bypass the CockroachDB retry and metrics wrappers.
func (d *DB) RawDB() *sqlx.DB {
	return d.db
}

// ExecuteTx runs fn inside a transaction with automatic retry on serialization
// failures. It uses the default isolation level and read-only settings from Config.
func (d *DB) ExecuteTx(ctx context.Context, fn func(*sqlx.Tx) error) error {
	return d.ExecuteTxWithOptions(ctx, fn, nil)
}

// ExecuteTxOptions overrides the DB-level transaction settings for a single
// [DB.ExecuteTxWithOptions] call. Nil fields fall back to the DB's [crdbgo.Config].
type ExecuteTxOptions struct {
	ReadOnly       *bool
	IsolationLevel *sql.IsolationLevel
}

// ExecuteTxWithOptions runs fn inside a transaction with automatic retry on
// serialization failures and per-call overrides for isolation level and
// read-only mode. Pass nil for txOpts to use the DB defaults.
func (d *DB) ExecuteTxWithOptions(ctx context.Context, fn func(*sqlx.Tx) error, txOpts *ExecuteTxOptions) error {
	start := time.Now()
	retries := 0

	isoLevel := d.sqlIsolationLevel()
	readOnly := d.config.ReadOnly
	if txOpts != nil {
		if txOpts.IsolationLevel != nil {
			isoLevel = *txOpts.IsolationLevel
		}
		if txOpts.ReadOnly != nil {
			readOnly = *txOpts.ReadOnly
		}
	}

	for {
		err := d.executeSingleTx(ctx, fn, isoLevel, readOnly)
		if err == nil {
			if d.metrics != nil {
				d.metrics.RecordTransaction(time.Since(start), retries, true)
			}
			return nil
		}

		if !IsRetryable(err) {
			if d.metrics != nil {
				d.metrics.RecordTransaction(time.Since(start), retries, false)
			}
			return err
		}

		if !d.config.Retry.ShouldRetry(retries) {
			if d.metrics != nil {
				d.metrics.RecordTransaction(time.Since(start), retries, false)
			}
			return err
		}

		delay := d.config.Retry.NextDelay(retries)
		if d.metrics != nil {
			d.metrics.RecordRetry(delay)
		}
		retries++

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// executeSingleTx runs a single transaction attempt.
func (d *DB) executeSingleTx(ctx context.Context, fn func(*sqlx.Tx) error, isoLevel sql.IsolationLevel, readOnly bool) error {
	tx, err := d.db.BeginTxx(ctx, &sql.TxOptions{
		Isolation: isoLevel,
		ReadOnly:  readOnly,
	})
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

// sqlIsolationLevel converts the crdbgo isolation level to database/sql's.
func (d *DB) sqlIsolationLevel() sql.IsolationLevel {
	switch d.config.IsolationLevel {
	case crdbgo.ReadCommitted:
		return sql.LevelReadCommitted
	default:
		return sql.LevelSerializable
	}
}

// QueryFollowerRead executes a query as a CockroachDB follower read by appending
// AS OF SYSTEM TIME follower_read_timestamp(). Returns [sqlx.Rows] for scanning.
func (d *DB) QueryFollowerRead(ctx context.Context, query string, args ...any) (*sqlx.Rows, error) {
	start := time.Now()
	fullQuery := fmt.Sprintf("%s %s", query, crdbgo.FollowerReadTimestamp())
	rows, err := d.db.QueryxContext(ctx, fullQuery, args...)
	if err == nil && d.metrics != nil {
		d.metrics.RecordFollowerRead(time.Since(start))
	}
	return rows, err
}

// QueryRowFollowerRead executes a query with follower read timestamp and
// returns a single row.
func (d *DB) QueryRowFollowerRead(ctx context.Context, query string, args ...any) *sqlx.Row {
	fullQuery := fmt.Sprintf("%s %s", query, crdbgo.FollowerReadTimestamp())
	return d.db.QueryRowxContext(ctx, fullQuery, args...)
}

// GetFollowerRead is like sqlx.Get but appends AS OF SYSTEM TIME
// follower_read_timestamp() to route the read to the nearest replica.
func (d *DB) GetFollowerRead(ctx context.Context, dest any, query string, args ...any) error {
	start := time.Now()
	fullQuery := fmt.Sprintf("%s %s", query, crdbgo.FollowerReadTimestamp())
	err := d.db.GetContext(ctx, dest, fullQuery, args...)
	if err == nil && d.metrics != nil {
		d.metrics.RecordFollowerRead(time.Since(start))
	}
	return err
}

// SelectFollowerRead is like sqlx.Select but appends AS OF SYSTEM TIME
// follower_read_timestamp() to route the read to the nearest replica.
func (d *DB) SelectFollowerRead(ctx context.Context, dest any, query string, args ...any) error {
	start := time.Now()
	fullQuery := fmt.Sprintf("%s %s", query, crdbgo.FollowerReadTimestamp())
	err := d.db.SelectContext(ctx, dest, fullQuery, args...)
	if err == nil && d.metrics != nil {
		d.metrics.RecordFollowerRead(time.Since(start))
	}
	return err
}

// compile-time interface check
var _ crdbgo.HealthChecker = (*DB)(nil)
