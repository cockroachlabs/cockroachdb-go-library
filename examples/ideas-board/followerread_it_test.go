package main

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/pgxadapter"
)

// tag::follower-read-it[]

// TestFollowerRead_TopIdeasFromSnapshot proves the follower-read leaderboard end to
// end: TopIdeas reads AS OF SYSTEM TIME follower_read_timestamp() and returns the
// ideas ordered by votes. Because a follower read is ~4.8s stale, freshly-seeded
// rows are invisible until wall-clock passes the staleness window, so the test
// polls until the snapshot catches up. Skips when Docker is unavailable.
func TestFollowerRead_TopIdeasFromSnapshot(t *testing.T) {
	cs, cleanup := startContainer(t, "cockroachdb/cockroach:latest-v25.1")
	defer cleanup()
	ctx := context.Background()

	pool, err := pgxadapter.NewPool(ctx, cs, []crdbgo.Option{})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	if err := ensureSchema(ctx, pool.RawPool()); err != nil {
		t.Fatalf("schema: %v", err)
	}
	for _, idea := range []Idea{
		{ID: "dark-mode", Text: "Dark mode", Votes: 5},
		{ID: "search", Text: "Full-text search", Votes: 12},
		{ID: "export", Text: "CSV export", Votes: 3},
	} {
		if err := seedIdea(ctx, pool.RawPool(), idea); err != nil {
			t.Fatalf("seed %s: %v", idea.ID, err)
		}
	}

	reads := NewIdeaReadService(pool)

	// Poll: the seeded rows appear once follower_read_timestamp() advances past
	// their insert time. An as-of read before the schema existed errors transiently
	// ("does not exist"); tolerate those while the window catches up.
	top := awaitTopIdeas(t, ctx, reads, 3, 30*time.Second)

	if len(top) != 3 {
		t.Fatalf("follower read returned %d ideas, want 3", len(top))
	}
	if top[0].Text != "Full-text search" || top[0].Votes != 12 {
		t.Fatalf("highest-voted idea must sort first, got %+v", top[0])
	}
	for i := 1; i < len(top); i++ {
		if top[i-1].Votes < top[i].Votes {
			t.Fatalf("leaderboard not ordered by votes desc: %+v", top)
		}
	}
	t.Logf("follower snapshot caught up: %+v", top)
}

// end::follower-read-it[]

func awaitTopIdeas(t *testing.T, ctx context.Context, reads *IdeaReadService, want int, timeout time.Duration) []Idea {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last []Idea
	var lastErr error
	for time.Now().Before(deadline) {
		got, err := reads.TopIdeas(ctx, 10)
		if err != nil {
			lastErr = err // as-of read before schema existed; retry as window advances
		} else {
			lastErr = nil
			last = got
			if len(got) >= want {
				return got
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("follower read never succeeded before deadline: %v", lastErr)
	}
	t.Fatalf("follower read never caught up: want %d rows, last saw %d", want, len(last))
	return last
}
