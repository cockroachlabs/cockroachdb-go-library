# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2026-10-28

Follower-read staleness is now validated and fails fast instead of emitting
invalid SQL, and custom staleness can be passed through every adapter.

### Added
- `crdbgo.FollowerReadClause(d time.Duration) (string, error)` — the single gate
  for building an `AS OF SYSTEM TIME` clause: `0` → `follower_read_timestamp()`,
  `> 0` → `'-Xs'`, `< 0` → error.
- Custom-staleness adapter methods: pgx `QueryFollowerReadAs` /
  `QueryRowFollowerReadAs`; sqlx `QueryFollowerReadAs` / `QueryRowFollowerReadAs` /
  `GetFollowerReadAs` / `SelectFollowerReadAs`. Previously the adapters could only
  issue follower reads at `follower_read_timestamp()`.

### Changed
- **BREAKING:** `crdbgo.FollowerReadWithStaleness(d)` now returns `(string, error)`
  (was `string`) and rejects `d <= 0`. The gorm `FollowerReadWithStaleness` scope
  now records the error via `db.AddError` on invalid staleness instead of injecting
  a malformed clause.

### Fixed
- A negative or zero duration previously produced invalid SQL such as
  `AS OF SYSTEM TIME '--10s'` / `'-0s'`. Invalid staleness now fails fast.

[0.2.0]: https://github.com/cockroachlabs/cockroachdb-go-library/releases/tag/v0.2.0
