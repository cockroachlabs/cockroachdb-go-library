package main

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/pgxadapter"
	"github.com/testcontainers/testcontainers-go/modules/cockroachdb"
)

// tag::concurrency-test[]

const (
	goroutines     = 3
	votesPerWorker = 20
	expectedVotes  = goroutines * votesPerWorker // 60
)

// startService boots a throwaway CockroachDB container and returns a ready
// Service. It skips the test cleanly when Docker is unavailable.
func startService(t *testing.T) (*Service, func()) {
	t.Helper()
	ctx := context.Background()

	container, err := cockroachdb.Run(ctx,
		"cockroachdb/cockroach:latest-v25.1",
		cockroachdb.WithInsecure(),
		cockroachdb.WithDatabase("ideasdb"),
	)
	if err != nil {
		t.Skipf("skipping integration test: cannot start CockroachDB container (Docker required): %v", err)
	}

	// NOTE: the testcontainers cockroachdb module v0.44 ConnectionString returns a
	// pgx stdlib registered-config KEY (for sql.Open), not a URL pgxpool can parse,
	// so we build the DSN from the container's host and mapped SQL port instead.
	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "26257/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("mapped port: %v", err)
	}
	cs := fmt.Sprintf("postgres://root@%s:%s/ideasdb?sslmode=disable", host, port.Port())

	pool, err := pgxadapter.NewPool(ctx, cs, []crdbgo.Option{
		crdbgo.WithMaxRetries(100),
	})
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("NewPool: %v", err)
	}

	svc := NewService(pool)
	if err := ensureSchema(ctx, pool.RawPool()); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if err := seedIdea(ctx, pool.RawPool(), Idea{ID: ideaID, Text: "Add dark mode", Votes: 0}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cleanup := func() {
		pool.Close()
		_ = container.Terminate(context.Background())
	}
	return svc, cleanup
}

// hammer runs goroutines × votesPerWorker concurrent upvotes and returns the
// number of failed calls.
func hammer(ctx context.Context, upvote func(context.Context, string) error) int {
	var wg sync.WaitGroup
	start := make(chan struct{})
	failures := make([]int, goroutines)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			for i := 0; i < votesPerWorker; i++ {
				if err := upvote(ctx, ideaID); err != nil {
					failures[idx]++
				}
			}
		}(g)
	}

	close(start)
	wg.Wait()

	total := 0
	for _, f := range failures {
		total += f
	}
	return total
}

func TestUpvote_Concurrent_AllVotesLand(t *testing.T) {
	svc, cleanup := startService(t)
	defer cleanup()
	ctx := context.Background()

	// Retry path: every one of the 60 upvotes must land, with zero errors.
	if err := reset(ctx, svc.pool.RawPool(), ideaID); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if failed := hammer(ctx, svc.Upvote); failed != 0 {
		t.Fatalf("Upvote (retry) returned %d errors, want 0", failed)
	}
	got, err := svc.Votes(ctx, ideaID)
	if err != nil {
		t.Fatalf("Votes: %v", err)
	}
	if got != expectedVotes {
		t.Fatalf("retry path: got %d votes, want exactly %d", got, expectedVotes)
	}

	// Plain path: under the same contention it drops votes to 40001 conflicts,
	// so it does NOT reliably reach the expected total.
	if err := reset(ctx, svc.pool.RawPool(), ideaID); err != nil {
		t.Fatalf("reset: %v", err)
	}
	_ = hammer(ctx, svc.UpvoteWithoutRetry)
	plain, err := svc.Votes(ctx, ideaID)
	if err != nil {
		t.Fatalf("Votes: %v", err)
	}
	if plain >= expectedVotes {
		t.Fatalf("plain path unexpectedly reached %d votes; expected lost updates below %d", plain, expectedVotes)
	}
	t.Logf("retry path landed %d/%d; plain path landed only %d/%d", got, expectedVotes, plain, expectedVotes)
}

// end::concurrency-test[]
