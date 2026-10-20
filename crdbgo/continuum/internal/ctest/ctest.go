// Package ctest provides shared helpers for Cockroach Continuum integration tests
// that run against the live CockroachDB node on localhost:26257. Tests use a
// dedicated "continuum_go" database so they never collide with other suites.
package ctest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testDatabase is the dedicated database all Continuum integration tests use.
const testDatabase = "continuum_go"

// baseDSN returns the admin DSN (pointing at defaultdb) used to bootstrap the
// test database. Override with CRDB_CONTINUUM_DSN.
func baseDSN() string {
	if v := os.Getenv("CRDB_CONTINUUM_DSN"); v != "" {
		return v
	}
	return "postgresql://root@localhost:26257/defaultdb?sslmode=disable"
}

// Pool returns a *pgxpool.Pool connected to the dedicated continuum_go database,
// creating the database first if needed. The pool is closed on test cleanup.
// Tests fail (not skip) if the node is unreachable — an integration node is
// expected to be running.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Bootstrap: create the dedicated database via the admin DSN.
	admin, err := pgxpool.New(ctx, baseDSN())
	if err != nil {
		t.Fatalf("ctest: connect admin pool: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE IF NOT EXISTS "+testDatabase); err != nil {
		admin.Close()
		t.Fatalf("ctest: create database %s: %v", testDatabase, err)
	}
	admin.Close()

	// Connect to the dedicated database by rewriting the DSN's database name.
	cfg, err := pgxpool.ParseConfig(baseDSN())
	if err != nil {
		t.Fatalf("ctest: parse DSN: %v", err)
	}
	cfg.ConnConfig.Database = testDatabase
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("ctest: connect %s pool: %v", testDatabase, err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ctest: ping %s: %v", testDatabase, err)
	}
	t.Cleanup(pool.Close)
	return pool
}

var counter atomic.Int64

// UniqueTable returns a collision-resistant table name with the given prefix,
// so parallel tests on the shared node do not interfere with each other.
func UniqueTable(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), counter.Add(1))
}

// ConnString ensures the dedicated database exists and returns a connection
// string pointing at it, suitable for pgxadapter.NewPool.
func ConnString(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, baseDSN())
	if err != nil {
		t.Fatalf("ctest: connect admin pool: %v", err)
	}
	_, err = admin.Exec(ctx, "CREATE DATABASE IF NOT EXISTS "+testDatabase)
	admin.Close()
	if err != nil {
		t.Fatalf("ctest: create database %s: %v", testDatabase, err)
	}

	u, err := url.Parse(baseDSN())
	if err != nil {
		t.Fatalf("ctest: parse base DSN: %v", err)
	}
	u.Path = "/" + testDatabase
	return u.String()
}
