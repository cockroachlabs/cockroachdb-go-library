package gormadapter

import (
	"fmt"
	"time"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FollowerRead returns a GORM scope that adds AS OF SYSTEM TIME
// follower_read_timestamp() to the query, enabling bounded-staleness reads
// from the nearest replica.
//
// Usage:
//
//	db.Scopes(gormadapter.FollowerRead()).Find(&orders)
func FollowerRead() func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Clauses(AsOfSystemTimeClause{
			SQL: crdbgo.FollowerReadTimestamp(),
		})
	}
}

// FollowerReadWithStaleness returns a GORM scope that adds AS OF SYSTEM TIME
// with a specific staleness interval.
//
// Usage:
//
//	db.Scopes(gormadapter.FollowerReadWithStaleness(10 * time.Second)).Find(&orders)
func FollowerReadWithStaleness(staleness time.Duration) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Clauses(AsOfSystemTimeClause{
			SQL: crdbgo.FollowerReadWithStaleness(staleness),
		})
	}
}

// InRegion returns a GORM scope that executes SET crdb_region before the query
// to route it to a specific CockroachDB region.
//
// Usage:
//
//	db.Scopes(gormadapter.InRegion("us-west-2")).Find(&orders)
func InRegion(region string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		stmt := fmt.Sprintf("SET crdb_region = '%s'", region)
		result := db.Session(&gorm.Session{NewDB: true}).Exec(stmt)
		if result.Error != nil {
			_ = db.AddError(result.Error)
		}
		return db
	}
}

// ReadCommitted returns a GORM scope that sets READ COMMITTED isolation level
// on the current transaction.
//
// Usage:
//
//	db.Scopes(gormadapter.ReadCommitted()).Create(&order)
func ReadCommitted() func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Set("crdbgo:isolation", crdbgo.ReadCommitted)
	}
}

// ReadOnly returns a GORM scope that marks the transaction as read-only.
//
// Usage:
//
//	db.Scopes(gormadapter.ReadOnly()).Find(&orders)
func ReadOnly() func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Set("crdbgo:readonly", true)
	}
}

// AsOfSystemTimeClause implements [clause.Expression] to inject CockroachDB's
// AS OF SYSTEM TIME clause into GORM-generated SQL. Use the [FollowerRead] or
// [FollowerReadWithStaleness] scopes rather than constructing this directly.
type AsOfSystemTimeClause struct {
	SQL string
}

// Build writes the clause SQL into the GORM query builder.
func (a AsOfSystemTimeClause) Build(builder clause.Builder) {
	_, _ = builder.WriteString(a.SQL)
}

// Name returns the clause name for GORM's clause registry.
func (a AsOfSystemTimeClause) Name() string {
	return "AS OF SYSTEM TIME"
}

// MergeClause handles clause merging when multiple AS OF SYSTEM TIME
// clauses are added. The last one wins.
func (a AsOfSystemTimeClause) MergeClause(c *clause.Clause) {
	c.Expression = a
}
