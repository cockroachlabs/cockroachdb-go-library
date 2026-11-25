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

## Learn CockroachDB fundamentals

The library handles the Go/pgx side; these CockroachDB docs explain the database underneath. The links
below go to the Markdown version of each page, which is easier for an agent to read; drop the `.md` for
the HTML page. Every docs page has one, and <https://docs.cockroachlabs.com/llms.txt> indexes the whole
documentation site.

**Start here**

- [Developer Basics](https://docs.cockroachlabs.com/docs/stable/developer-basics.md): what's different about building on CockroachDB.
- [PostgreSQL Compatibility](https://docs.cockroachlabs.com/docs/stable/postgresql-compatibility.md): CockroachDB speaks the PostgreSQL wire protocol (use the pgx driver / `lib/pq`), with some differences.
- [Connection Pooling](https://docs.cockroachlabs.com/docs/stable/connection-pooling.md): sizing `pgxpool` (the pool behind the pgx adapter).
- [Common Errors and Solutions](https://docs.cockroachlabs.com/docs/stable/common-errors.md)

**Transactions and contention**

- [Transactions](https://docs.cockroachlabs.com/docs/stable/transactions.md): SERIALIZABLE by default, and why clients see retry errors.
- [Transaction Retry Error Reference](https://docs.cockroachlabs.com/docs/stable/transaction-retry-error-reference.md): what each `40001` (`RETRY_SERIALIZABLE`, `RETRY_WRITE_TOO_OLD`, ...) means and how to make it rarer. `ExecuteTx` retries them; this tells you why they happen.
- [Reduce transaction contention](https://docs.cockroachlabs.com/docs/stable/performance-best-practices-overview.md#reduce-transaction-contention): the checklist. Keep transactions small, limit the rows they touch, use `SELECT ... FOR UPDATE` for read-then-write, `UPSERT` whole rows, use historical reads where staleness is fine.
- [Understand Hotspots](https://docs.cockroachlabs.com/docs/stable/understand-hotspots.md): ranges that take far more traffic than others, and how to spread it.
- [Performance Tuning Recipes](https://docs.cockroachlabs.com/docs/stable/performance-recipes.md): symptoms and fixes, contention included.
- [Monitor and Analyze Transaction Contention](https://docs.cockroachlabs.com/docs/stable/monitor-and-analyze-transaction-contention.md)
- [`SELECT FOR UPDATE`](https://docs.cockroachlabs.com/docs/stable/select-for-update.md)
- [Read Committed Transactions](https://docs.cockroachlabs.com/docs/stable/read-committed.md): an alternative isolation level, with trade-offs. Don't switch to it just to avoid retries (it hides `40001`, so the adapter's retry never fires).

**Schema design**

- [Database Schemas](https://docs.cockroachlabs.com/docs/stable/schema-design-overview.md): how to design and change schemas, including why to change them with a migration tool rather than through an ORM.
- [Unique ID best practices](https://docs.cockroachlabs.com/docs/stable/performance-best-practices-overview.md#unique-id-best-practices): prefer `UUID` keys (`gen_random_uuid()`) over sequences or auto-increment, which create hotspots.
- [`UUID`](https://docs.cockroachlabs.com/docs/stable/uuid.md) and [Hash-sharded Indexes](https://docs.cockroachlabs.com/docs/stable/hash-sharded-indexes.md), for when an index must be on a sequential column.
- [Online Schema Changes](https://docs.cockroachlabs.com/docs/stable/online-schema-changes.md): schema changes run in the background; what that means for migrations.

**Multi-region**

- [Multi-Region Capabilities Overview](https://docs.cockroachlabs.com/docs/stable/multiregion-overview.md)
- [Table Localities](https://docs.cockroachlabs.com/docs/stable/table-localities.md): `REGIONAL BY TABLE`, `REGIONAL BY ROW` and `GLOBAL`. For `REGIONAL BY ROW`, home rows by writing the `crdb_region` column from the active `RegionContext`.
- [How to Choose a Multi-Region Configuration](https://docs.cockroachlabs.com/docs/stable/choosing-a-multi-region-configuration.md)
- [Follower Reads](https://docs.cockroachlabs.com/docs/stable/follower-reads.md) and [`AS OF SYSTEM TIME`](https://docs.cockroachlabs.com/docs/stable/as-of-system-time.md), behind `QueryFollowerRead` / `FollowerReadClause`.

**Change data capture**

- [`CREATE CHANGEFEED`](https://docs.cockroachlabs.com/docs/stable/create-changefeed.md) and [Changefeed Sinks](https://docs.cockroachlabs.com/docs/stable/changefeed-sinks.md) (the webhook sink is what the `cdc` package receives).

**Vector search**

- [`VECTOR`](https://docs.cockroachlabs.com/docs/stable/vector.md) and [Vector Indexes](https://docs.cockroachlabs.com/docs/stable/vector-indexes.md) (behind the `ai` package).
