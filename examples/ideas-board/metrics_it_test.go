package main

import (
	"context"
	"testing"
)

// TestMetrics_CountersMoveUnderContention proves the metrics module counts real
// work: under contention on one row the retrying Upvote fires 40001 retries and
// records one transaction per call. The InMemoryRecorder's snapshot must show both
// counters moved. Skips when Docker is unavailable.
func TestMetrics_CountersMoveUnderContention(t *testing.T) {
	cs, cleanup := startContainer(t, "cockroachdb/cockroach:latest-v25.1")
	defer cleanup()
	ctx := context.Background()

	svc, rec, err := newMeteredService(ctx, cs)
	if err != nil {
		t.Fatalf("newMeteredService: %v", err)
	}
	defer svc.pool.Close()

	if err := ensureSchema(ctx, svc.pool.RawPool()); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if err := seedIdea(ctx, svc.pool.RawPool(), Idea{ID: ideaID, Text: "Instrument the board", Votes: 0}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := reset(ctx, svc.pool.RawPool(), ideaID); err != nil {
		t.Fatalf("reset: %v", err)
	}

	// tag::metrics-it[]
	// Each Upvote is one ExecuteTx; the pool records a transaction per call and a
	// retry per absorbed 40001 conflict — automatically, via the attached recorder.
	if failed := hammer(ctx, svc.Upvote); failed != 0 {
		t.Fatalf("retry must absorb every conflict, got %d failures", failed)
	}

	snap := rec.Snapshot()
	if snap.TransactionCount != int64(expectedVotes) {
		t.Fatalf("TransactionCount = %d, want %d (one per upvote)", snap.TransactionCount, expectedVotes)
	}
	if snap.TransactionSuccessCount != int64(expectedVotes) {
		t.Fatalf("TransactionSuccessCount = %d, want %d", snap.TransactionSuccessCount, expectedVotes)
	}
	if snap.RetryCount == 0 {
		t.Fatalf("RetryCount = 0; contention on one row must force at least one 40001 retry")
	}
	// end::metrics-it[]

	got, err := svc.Votes(ctx, ideaID)
	if err != nil {
		t.Fatalf("Votes: %v", err)
	}
	if got != expectedVotes {
		t.Fatalf("votes landed = %d, want %d", got, expectedVotes)
	}
	t.Logf("recorded %d transactions, %d retries; %d votes landed",
		snap.TransactionCount, snap.RetryCount, got)
}
