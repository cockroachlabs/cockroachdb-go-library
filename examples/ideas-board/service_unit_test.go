package main

import (
	"context"
	"testing"
)

// tag::unit-test[]

// fakeStore is an in-memory voteStore for fast, Docker-free unit testing.
type fakeStore struct {
	votes int
}

func (f *fakeStore) CurrentVotes(_ context.Context, _ string) (int, error) { return f.votes, nil }
func (f *fakeStore) UpdateVotes(_ context.Context, _ string, votes int) error {
	f.votes = votes
	return nil
}

// TestApplyUpvote_IncrementsByOne exercises the pure read-increment-save logic
// with a fake store — no database, no container.
func TestApplyUpvote_IncrementsByOne(t *testing.T) {
	store := &fakeStore{votes: 41}

	if err := applyUpvote(context.Background(), store, ideaID); err != nil {
		t.Fatalf("applyUpvote: %v", err)
	}

	if store.votes != 42 {
		t.Fatalf("expected votes 42, got %d", store.votes)
	}
}

// end::unit-test[]
