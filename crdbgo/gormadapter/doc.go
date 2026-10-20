// Package gormadapter provides CockroachDB integration for GORM via the
// [gorm.Plugin] interface.
//
// It adds automatic transaction retry on serialization failures (SQLSTATE 40001 /
// CR000) with exponential backoff, follower read scopes via AS OF SYSTEM TIME,
// multi-region query routing, and optional metrics recording.
//
// Basic usage:
//
//	import (
//	    "github.com/felipeg48/cockroachdb-go-library/crdbgo"
//	    "github.com/felipeg48/cockroachdb-go-library/crdbgo/gormadapter"
//	)
//
//	db, _ := gorm.Open(postgres.Open(dsn), &gorm.Config{})
//	db.Use(gormadapter.NewPlugin(
//	    []crdbgo.Option{crdbgo.WithMaxRetries(5)},
//	    gormadapter.WithMetrics(recorder),
//	))
//
//	// Follower read scope
//	db.Scopes(gormadapter.FollowerRead()).Find(&orders)
//
//	// Region-specific query
//	db.Scopes(gormadapter.InRegion("us-west-2")).Find(&orders)
//
//	// Read committed isolation
//	db.Scopes(gormadapter.ReadCommitted()).Create(&order)
package gormadapter
