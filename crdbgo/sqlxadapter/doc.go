// Package sqlxadapter integrates CockroachDB features with jmoiron/sqlx.
//
// It wraps sqlx.DB with automatic transaction retries, follower reads,
// health checking, and metrics recording using the core crdbgo configuration.
//
// Usage:
//
//	db, err := sqlxadapter.Open("postgres://...", []crdbgo.Option{
//	    crdbgo.WithMaxRetries(5),
//	    crdbgo.WithRegion("us-east-1"),
//	}, sqlxadapter.WithMetrics(recorder))
//
//	err = db.ExecuteTx(ctx, func(tx *sqlx.Tx) error {
//	    _, err := tx.ExecContext(ctx, "INSERT INTO accounts (id, balance) VALUES ($1, $2)", id, balance)
//	    return err
//	})
//
// The package uses pgx/v5/stdlib as the underlying database/sql driver,
// which gives access to pgx's error types for precise SQLSTATE detection.
package sqlxadapter
