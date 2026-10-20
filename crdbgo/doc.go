// Package crdbgo provides CockroachDB-specific features for Go applications:
// automatic transaction retry with exponential backoff (SQLSTATE 40001 / CR000),
// multi-region routing, follower reads via AS OF SYSTEM TIME, composable
// middleware, and circuit breaker integration.
//
// Configuration uses the functional options pattern:
//
//	cfg := crdbgo.NewConfig(
//	    crdbgo.WithMaxRetries(5),
//	    crdbgo.WithFollowerReads(10 * time.Second),
//	    crdbgo.WithRegion("us-east-1"),
//	)
//
// Use the adapter packages (pgxadapter, gormadapter, sqlxadapter) to integrate
// with your database driver. Use crdbtest for testing utilities and crdbgo/ai
// for vector search with CockroachDB's native VECTOR type.
package crdbgo
