// Package pgxadapter integrates CockroachDB features with the pgx/v5 driver.
//
// It wraps [pgxpool.Pool] with automatic transaction retries on serialization
// failures (SQLSTATE 40001 / CR000), follower reads via AS OF SYSTEM TIME,
// health checking, and optional metrics recording. Configure via [crdbgo.Option]
// values passed to [NewPool]:
//
//	pool, err := pgxadapter.NewPool(ctx, connString,
//	    []crdbgo.Option{crdbgo.WithMaxRetries(5)},
//	    pgxadapter.WithMetrics(recorder),
//	)
package pgxadapter
