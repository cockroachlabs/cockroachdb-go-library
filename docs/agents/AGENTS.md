# AGENTS.md — cockroachdb-go-library (consumer guide)

Guidance for coding agents writing a **Go app that imports this library**. It is not a guide to
the library's own repo. Module: `github.com/cockroachlabs/cockroachdb-go-library`. Install:
`go get github.com/cockroachlabs/cockroachdb-go-library@latest`. See the [README](../../README.md)
for full detail — this file is the short version; every symbol below is real.

## Already handled — do not rebuild it

- **Serialization-retry on SQLSTATE `40001`.** The adapter transaction wrappers retry with
  exponential backoff + jitter automatically. Wrap your work in `pool.ExecuteTx(ctx, func(tx pgx.Tx) error {...})`
  (pgx adapter) — or the sqlx/GORM equivalents. **Do NOT hand-roll a retry loop.** Tune via
  `crdbgo.WithMaxRetries(n)` (default 5) and friends. Per-tx overrides go through
  `pool.ExecuteTxWithOptions(ctx, fn, &pgxadapter.ExecuteTxOptions{...})`.
- **Error classification.** Use `crdbgo.IsRetryable` / `pgxadapter.IsRetryable` /
  `crdbgo.IsSerializationFailure` — do not string-match SQLSTATE yourself.
- **Metrics, CDC plumbing, Continuum, circuit breaker** are provided (`metrics`, `cdc`,
  `continuum/*`, `circuitbreaker` packages). Don't reimplement them.

## Which helper for each job

| Job | Use |
|-----|-----|
| Transaction w/ auto-retry | `pgxadapter.NewPool(...).ExecuteTx` (also `sqlxadapter`, `gormadapter`) |
| Follower read (stale, nearest replica) | `pool.QueryFollowerRead` / `QueryFollowerReadAs(d)` / `QueryRowFollowerRead*`; build SQL yourself with `crdbgo.FollowerReadClause(d)` |
| Region context | `crdbgo.WithRegionContext(ctx, &crdbgo.RegionContext{Region: "us-east-1"})`, read back `crdbgo.RegionFromContext`; resolver chains via `crdbgo.ChainRegionResolver` / `StaticRegionResolver` |
| Regional-by-row homing | Write the **`crdb_region` COLUMN** (set it from the active `RegionContext`). The GORM adapter stamps it on insert. **Never** issue a `SET crdb_region` statement — no such session var exists. |
| CDC / changefeed | `cdc` package (changefeed SQL builder, webhook handler, manager) |
| AI / vector search / RAG | `ai` package (`NewVectorSearchTemplate`, `NewVectorStore`; native `VECTOR` + C-SPANN) |
| Continuum / MCP | `continuum/cloud`, `continuum/mcp`, `continuum/aegis` (simulated until an API key/endpoint is set) |
| Health check | `crdbgo.NewHealthHandler(pool)` |

## Config that matters

Pass `[]crdbgo.Option` to the adapter constructor: `WithMaxRetries`, `WithInitialDelay`,
`WithMaxDelay`, `WithMultiplier`, `WithJitter`, `WithRegion`, `WithFollowerReads(d)` (staleness),
`WithIsolationLevel(crdbgo.Serializable | crdbgo.ReadCommitted)`, `WithReadOnly`.

## Anti-patterns

- Hand-rolled `40001` retry loop — `ExecuteTx` already does it.
- Raw `SET crdb_region` — home rows by writing the `crdb_region` column instead.
- Writing changefeed plumbing by hand — use the `cdc` package.
- `ReadCommitted` hides `40001` (retries never fire). Default is `Serializable`; keep it unless you
  know you want RC.
- A negative follower-read staleness is rejected (never silently reads current data) — pass `0` for
  the default `follower_read_timestamp()`.

Not applicable in Go: there is no declarative multi-region annotation/startup check, and no
`X-CockroachDB-Region` web header — those are web-framework concerns the Go library does not own.

## Testing

Use the `crdbtest` package: `NewHarness(t, crdbtest.WithConnectionString(...), crdbtest.WithSchema(...))`
for a live cluster, or `NewRetrySimulator` / `NewConcurrencyRunner` / `NewFollowerReadSimulator` for
no-cluster tests. Integration tests use Testcontainers for Go (Docker required).
