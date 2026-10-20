//go:build integration

// Package testutil provides shared helpers for integration tests that
// require a real CockroachDB instance via Testcontainers.
package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jmoiron/sqlx"
	"github.com/testcontainers/testcontainers-go/modules/cockroachdb"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx stdlib driver for database/sql
)

var (
	once    sync.Once
	connStr string
	initErr error
)

// StartCockroachDB starts a single CockroachDB container shared across all
// integration tests in the process. The container is started in insecure mode.
// Call this from TestMain or at the top of each test.
func StartCockroachDB(ctx context.Context) (string, error) {
	once.Do(func() {
		container, err := cockroachdb.Run(ctx,
			"cockroachdb/cockroach:latest-v25.1",
			cockroachdb.WithInsecure(),
			cockroachdb.WithDatabase("testdb"),
		)
		if err != nil {
			initErr = fmt.Errorf("testutil: start container: %w", err)
			return
		}
		cs, err := container.ConnectionString(ctx)
		if err != nil {
			initErr = fmt.Errorf("testutil: connection string: %w", err)
			return
		}
		connStr = cs
	})
	return connStr, initErr
}

// ConnString returns the connection string, starting the container if needed.
func ConnString(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	cs, err := StartCockroachDB(context.Background())
	if err != nil {
		t.Fatalf("testutil: %v", err)
	}
	return cs
}

// PgxPool returns a *pgxpool.Pool connected to the shared container.
func PgxPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cs := ConnString(t)
	pool, err := pgxpool.New(context.Background(), cs)
	if err != nil {
		t.Fatalf("testutil: pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// SqlDB returns a *sql.DB connected via the pgx stdlib driver.
func SqlDB(t *testing.T) *sql.DB {
	t.Helper()
	cs := ConnString(t)
	db, err := sql.Open("pgx", cs)
	if err != nil {
		t.Fatalf("testutil: sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// SqlxDB returns a *sqlx.DB connected via the pgx stdlib driver.
func SqlxDB(t *testing.T) *sqlx.DB {
	t.Helper()
	cs := ConnString(t)
	db, err := sqlx.Open("pgx", cs)
	if err != nil {
		t.Fatalf("testutil: sqlx.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// GormDB returns a *gorm.DB connected via the postgres (pgx) driver.
func GormDB(t *testing.T) *gorm.DB {
	t.Helper()
	cs := ConnString(t)
	db, err := gorm.Open(postgres.Open(cs), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("testutil: gorm.Open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("testutil: gorm underlying sql.DB: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

// UniqueTable returns a table name with a random suffix to avoid test collision.
func UniqueTable(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, uniqueCounter())
}

var (
	counterMu sync.Mutex
	counter   int64
)

func uniqueCounter() int64 {
	counterMu.Lock()
	defer counterMu.Unlock()
	counter++
	return counter
}
