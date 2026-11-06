# Concurrent Ideas Board (Go)

A runnable demo of the hero feature of `cockroachdb-go-library`: **automatic
retry on serialization conflicts (SQLSTATE 40001)** via `(*pgxadapter.Pool).ExecuteTx`.

Many users upvote the same idea at once. That is a classic write-contention
hotspot: concurrent read-modify-write on one row under `SERIALIZABLE` isolation
makes CockroachDB abort the losers with error `40001`. This example proves that:

- the **plain** path (`UpvoteWithoutRetry`, a hand-rolled transaction with no
  retry) *loses* votes — every aborted transaction is a dropped upvote; while
- the **retry** path (`Upvote`, wrapped in `ExecuteTx`) lands *every* vote,
  because the library transparently re-runs the transaction on `40001`.

This is the Go twin of the Spring / .NET / Java fundamentals demos.

## What it shows

| File | Purpose |
|------|---------|
| `idea.go` | The `Idea` entity (`ID`, `Text`, `Votes`). |
| `repository.go` | pgx read-modify-write helpers (`find`, `currentVotes`, `updateVotes`, `reset`). |
| `service.go` | `UpvoteWithoutRetry` (plain tx) vs `Upvote` (`ExecuteTx` retry). |
| `main.go` | Seeds one idea, fires 50 concurrent upvotes down each path, prints the result. |
| `service_unit_test.go` | Fast unit test of the pure increment logic (no Docker). |
| `service_concurrency_test.go` | Integration test: 3 × 20 concurrent upvotes against a real cluster. |

## Prerequisites

- Go 1.26+
- For `main.go`: a reachable CockroachDB (set `CRDB_URL`, defaults to
  `postgres://root@localhost:26257/defaultdb?sslmode=disable`).
- For the integration test: Docker (Testcontainers starts a throwaway cluster).

## Run it

```sh
# start a local insecure node (one option)
cockroach start-single-node --insecure --listen-addr=localhost:26257 --background

go run .
```

Example output:

```
fired 50 concurrent upvotes on one idea

plain  (no retry): 11/50 landed, 39 lost to 40001 conflicts
retry  (ExecuteTx): 50/50 landed, 0 errors
```

## Test it

```sh
go test ./...            # unit (always) + integration (needs Docker)
go test -short ./...     # unit only, no container
```

The unit test needs no database. The concurrency test boots a CockroachDB
container via Testcontainers and **skips cleanly** if Docker is unavailable.

On rootless Docker / Podman setups where the Testcontainers reaper (Ryuk)
cannot create its helper container (e.g. `network bridge not found`), disable it:

```sh
TESTCONTAINERS_RYUK_DISABLED=true go test ./...
```

> The test builds its DSN from the container's host + mapped SQL port rather
> than `container.ConnectionString`, because the cockroachdb module (v0.44)
> returns a pgx stdlib *registered-config key* (for `sql.Open`), not a URL that
> `pgxpool` can parse.

## About the `replace` directive

`go.mod` requires the published library at `v0.2.0` but adds:

```
replace github.com/cockroachlabs/cockroachdb-go-library => ../../
```

because this example lives inside the library's own repo and builds against the
local checkout. **Real consumers delete the `replace`** and pull `v0.2.0` from
the module proxy / git — nothing else changes.
