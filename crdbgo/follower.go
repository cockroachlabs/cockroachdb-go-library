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
func FollowerReadWithStaleness(staleness time.Duration) string {
	return fmt.Sprintf("AS OF SYSTEM TIME '-%s'", staleness)
}

// AsOfSystemTime returns an AS OF SYSTEM TIME clause for an exact timestamp,
// useful for historical reads at a specific point in time.
func AsOfSystemTime(t time.Time) string {
	return fmt.Sprintf("AS OF SYSTEM TIME '%s'", t.Format(time.RFC3339Nano))
}
