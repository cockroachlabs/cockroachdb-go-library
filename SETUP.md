# Setup, Testing & Demo Guide

This guide covers everything you need to get `cockroachdb-go-library` running: prerequisites, running the test suite, using it in your own project, and a walkthrough demo showing all features end-to-end.

## Prerequisites

| Requirement | Minimum Version | Check Command |
|-------------|----------------|---------------|
| **Go** | 1.22+ | `go version` |
| **CockroachDB** | 24.2+ (for VECTOR type) | `cockroach version` |
| **Git** | any | `git --version` |

### Installing Go

```bash
# macOS (Homebrew)
brew install go

# Linux (official binary)
wget https://go.dev/dl/go1.22.4.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.4.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

### Installing CockroachDB

```bash
# macOS (Homebrew)
brew install cockroachdb/tap/cockroach

# Linux
curl https://binaries.cockroachdb.com/cockroach-v24.2.0.linux-amd64.tgz | tar -xz
sudo cp cockroach-v24.2.0.linux-amd64/cockroach /usr/local/bin/

# Docker (alternative)
docker pull cockroachdb/cockroach:latest
```

## 1. Running the Unit Tests (No CockroachDB Needed)

All 247 unit tests run without a live database. The `crdbtest` simulators mock retryable errors, follower reads, and concurrency behavior.

```bash
# Clone the repository
git clone https://github.com/felipeg48/cockroachdb-go-library.git
cd cockroachdb-go-library

# Download dependencies
go mod download

# Run all unit tests
go test ./...

# With race detector (recommended)
go test -race ./...

# Verbose output showing each test
go test -v ./...
```

### Test breakdown by package

```bash
# Core library (config, retry, errors, region, follower, health, middleware, isolation)
go test -v ./crdbgo/...

# pgx adapter (error detection, pool, ExecuteTx, follower reads)
go test -v ./crdbgo/pgxadapter/...

# GORM adapter (plugin, scopes, callbacks, retry)
go test -v ./crdbgo/gormadapter/...

# sqlx adapter (DB wrapper, ExecuteTx, follower reads, health check)
go test -v ./crdbgo/sqlxadapter/...

# Circuit breaker (three-state, middleware, state callbacks)
go test -v ./crdbgo/circuitbreaker/...

# CDC (changefeed SQL builder, webhook handler, metrics)
go test -v ./crdbgo/cdc/...

# Metrics (InMemoryRecorder, OTelRecorder, Snapshot, concurrency)
go test -v ./crdbgo/metrics/...

# Testing utilities (ConcurrencyRunner, RetrySimulator, FollowerReadSimulator)
go test -v ./crdbgo/crdbtest/...

# AI / Vector Search / RAG (VectorSearchTemplate, VectorStore, RagChatClient)
go test -v ./crdbgo/ai/...
```

## 2. Integration Tests (Testcontainers — Docker Required)

Integration tests use [Testcontainers for Go](https://golang.testcontainers.org/) to automatically spin up a real CockroachDB container. No manual cluster setup needed — just Docker.

```bash
# Run integration tests (Docker must be running)
go test -tags=integration ./...

# With race detector
go test -tags=integration -race ./...

# Specific package
go test -tags=integration -v ./crdbgo/pgxadapter/...
go test -tags=integration -v ./crdbgo/ai/...
```

Integration tests are guarded by `//go:build integration` and cover 5 packages:

| Package | Tests | Covers |
|---------|-------|--------|
| `pgxadapter` | 5 | Pool creation, Ping, Check, ExecuteTx commit, concurrent retry, follower reads |
| `sqlxadapter` | 5 | Open, Check, ExecuteTx commit, QueryFollowerRead, Get/SelectFollowerRead |
| `gormadapter` | 4 | Plugin registration, CRUD, FollowerRead scope, concurrent write retry |
| `ai` | 8 | InitSchema, StoreEmbedding+Search (L2/Cosine/IP), VectorStore CRUD, metadata |
| `circuitbreaker` | 3 | Execute wrapping real DB, circuit opens after failures, recovery |

The shared test container helper lives in `crdbgo/testutil/container.go` and provides `ConnString()`, `PgxPool()`, `SqlDB()`, `SqlxDB()`, `GormDB()`, and `UniqueTable()` helpers.

## 3. Using the Library in Your Project

### Step 1: Add the dependency

```bash
mkdir myapp && cd myapp
go mod init myapp
go get github.com/felipeg48/cockroachdb-go-library
```

### Step 2: Import only what you need

```go
import (
	"github.com/felipeg48/cockroachdb-go-library/crdbgo"            // Core types
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/pgxadapter" // pgx adapter
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/metrics"    // Metrics
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/crdbtest"   // Testing (test files only)
)
```

### Step 3: Connect and use

```go
ctx := context.Background()

pool, err := pgxadapter.NewPool(ctx,
	"postgresql://root@localhost:26257/mydb?sslmode=disable",
	[]crdbgo.Option{
		crdbgo.WithMaxRetries(5),
		crdbgo.WithIsolationLevel(crdbgo.Serializable),
	},
)
if err != nil {
	log.Fatal(err)
}
defer pool.Close()
```

### Step 4: Run transactions with automatic retry

```go
err = pool.ExecuteTx(ctx, func(tx pgx.Tx) error {
	// This entire function is retried on SQLSTATE 40001
	_, err := tx.Exec(ctx, "INSERT INTO orders (id, total) VALUES ($1, $2)", orderID, total)
	return err
})
```

## 4. Demo App (Docker Compose)

The demo HTTP server showcases all library features. The easiest way to run it is via Docker Compose:

```bash
docker compose up --build
```

This starts three services:
1. **cockroachdb** — single-node CockroachDB cluster (ports 26257 + 8081 for DB Console)
2. **cockroach-init** — runs `deploy/init.sql` to seed schema + data
3. **demo** — the Go HTTP server on port 8080

### Available endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check (200 up / 503 down) |
| `GET` | `/metrics` | Metrics snapshot (JSON) |
| `POST` | `/tx/insert` | Insert account with automatic retry |
| `POST` | `/tx/transfer` | Transfer between accounts (serializable) |
| `GET` | `/follower-read` | Follower read query |
| `GET` | `/accounts` | List all accounts |
| `POST` | `/webhook/cdc` | CDC webhook endpoint |
| `GET` | `/circuit-breaker/status` | Circuit breaker state |
| `POST` | `/circuit-breaker/test` | Trip the circuit breaker |

### Try it

```bash
# Insert an account
curl -X POST http://localhost:8080/tx/insert \
  -H 'Content-Type: application/json' \
  -d '{"name":"Diana","balance":500}'

# Transfer funds
curl -X POST http://localhost:8080/tx/transfer \
  -H 'Content-Type: application/json' \
  -d '{"from":"<alice-uuid>","to":"<bob-uuid>","amount":100}'

# Follower read
curl http://localhost:8080/follower-read

# Check metrics
curl http://localhost:8080/metrics

# Trip the circuit breaker (call 3+ times)
curl -X POST http://localhost:8080/circuit-breaker/test
curl http://localhost:8080/circuit-breaker/status
```

### Running without Docker Compose

```bash
# Start CockroachDB locally
cockroach start-single-node --insecure --listen-addr=localhost:26257 --background

# Seed data
cockroach sql --insecure < deploy/init.sql

# Run the demo
go run ./cmd/demo/
```

## 5. Cleanup

```bash
# Stop CockroachDB
cockroach quit --insecure

# Remove data
rm -rf cockroach-data

# Or with Docker
docker stop cockroach && docker rm cockroach
```

## Troubleshooting

| Problem | Solution |
|---------|----------|
| `go: module requires Go >= 1.22` | Upgrade Go: `brew upgrade go` or download from https://go.dev/dl/ |
| `connection refused on :26257` | Start CockroachDB: `cockroach start-single-node --insecure --background` |
| `pq: database "testdb" does not exist` | Create it: `cockroach sql --insecure -e "CREATE DATABASE testdb"` |
| `40001 serialization_failure` | Expected under contention. The library retries automatically. |
| `go mod download` fails | Check network, try `GOPROXY=direct go mod download` |
| Race detector reports | Run `go test -race ./...` to verify. All 247 tests should pass race-free. |
| Integration tests fail | Ensure Docker is running. Testcontainers needs Docker to spin up CockroachDB. |
| `docker compose up` fails | Check port 26257/8080/8081 are free. Run `docker compose down` first. |
