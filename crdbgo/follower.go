package crdbgo

import (
	"fmt"
	"time"
)

// FollowerReadTimestamp returns the SQL clause for bounded-staleness follower
// reads using CockroachDB's built-in follower_read_timestamp() function.
// CockroachDB automatically picks the optimal staleness, routing reads to
// the nearest replica instead of requiring a round-trip to the leaseholder.
func FollowerReadTimestamp() string {
	return "AS OF SYSTEM TIME follower_read_timestamp()"
}

// FollowerReadWithStaleness returns an AS OF SYSTEM TIME clause with a specific
// staleness interval (e.g., 10*time.Second produces "AS OF SYSTEM TIME '-10s'").
//
// staleness must be positive: a follower read always reads into the past. A zero
// duration would produce "'-0s'" (a current read) and a negative duration would
// produce invalid SQL like "'--10s'" — both silently defeat the follower read, so
// they are rejected with an error rather than emitting a bad clause. Use
// [FollowerReadTimestamp] to let CockroachDB pick the staleness, or
// [FollowerReadClause] when a zero value should fall back to that default.
func FollowerReadWithStaleness(staleness time.Duration) (string, error) {
	if staleness <= 0 {
		return "", fmt.Errorf("crdbgo: follower read staleness must be positive, got %s", staleness)
	}
	return fmt.Sprintf("AS OF SYSTEM TIME '-%s'", staleness), nil
}

// FollowerReadClause returns the AS OF SYSTEM TIME clause for a follower read at
// the given staleness, applying the convention adapters expose to callers:
//
//   - staleness == 0: use follower_read_timestamp() (let CockroachDB decide).
//   - staleness  > 0: read exactly that far into the past (e.g. "'-10s'").
//   - staleness  < 0: rejected with an error — never silently reads current data.
//
// This is the single gate every adapter "...As" method routes through, so the
// zero-means-default and negative-is-an-error contract lives in one place.
func FollowerReadClause(staleness time.Duration) (string, error) {
	switch {
	case staleness == 0:
		return FollowerReadTimestamp(), nil
	case staleness < 0:
		return "", fmt.Errorf("crdbgo: follower read staleness must not be negative, got %s", staleness)
	default:
		return FollowerReadWithStaleness(staleness)
	}
}

// AsOfSystemTime returns an AS OF SYSTEM TIME clause for an exact timestamp,
// useful for historical reads at a specific point in time.
func AsOfSystemTime(t time.Time) string {
	return fmt.Sprintf("AS OF SYSTEM TIME '%s'", t.Format(time.RFC3339Nano))
}
