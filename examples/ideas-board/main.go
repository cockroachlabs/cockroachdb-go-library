package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/pgxadapter"
)

const ideaID = "idea-1"

// tag::runner[]

// runConcurrent fires n goroutines that all upvote the same idea at once. A
// closed channel acts as a start barrier so every goroutine races on the same
// row simultaneously, maximizing contention. It returns how many calls errored.
func runConcurrent(ctx context.Context, n int, upvote func(context.Context, string) error) int {
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start // wait on the barrier
			errs[idx] = upvote(ctx, ideaID)
		}(i)
	}

	close(start) // release all goroutines at once
	wg.Wait()

	failed := 0
	for _, err := range errs {
		if err != nil {
			failed++
		}
	}
	return failed
}

func main() {
	ctx := context.Background()

	connString := os.Getenv("CRDB_URL")
	if connString == "" {
		connString = "postgres://root@localhost:26257/defaultdb?sslmode=disable"
	}

	pool, err := pgxadapter.NewPool(ctx, connString, []crdbgo.Option{
		crdbgo.WithMaxRetries(10),
	})
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	svc := NewService(pool)
	if err := ensureSchema(ctx, pool.RawPool()); err != nil {
		log.Fatalf("schema: %v", err)
	}
	if err := seedIdea(ctx, pool.RawPool(), Idea{ID: ideaID, Text: "Add dark mode", Votes: 0}); err != nil {
		log.Fatalf("seed: %v", err)
	}

	const votes = 50

	// Plain path: no retry. Serialization conflicts are returned as errors and
	// those votes are lost.
	if err := reset(ctx, pool.RawPool(), ideaID); err != nil {
		log.Fatalf("reset: %v", err)
	}
	plainFailed := runConcurrent(ctx, votes, svc.UpvoteWithoutRetry)
	plainTotal, _ := svc.Votes(ctx, ideaID)

	// Retry path: ExecuteTx retries 40001 conflicts. Every vote lands.
	if err := reset(ctx, pool.RawPool(), ideaID); err != nil {
		log.Fatalf("reset: %v", err)
	}
	retryFailed := runConcurrent(ctx, votes, svc.Upvote)
	retryTotal, _ := svc.Votes(ctx, ideaID)

	fmt.Printf("fired %d concurrent upvotes on one idea\n\n", votes)
	fmt.Printf("plain  (no retry): %d/%d landed, %d lost to 40001 conflicts\n", plainTotal, votes, plainFailed)
	fmt.Printf("retry  (ExecuteTx): %d/%d landed, %d errors\n", retryTotal, votes, retryFailed)
}

// end::runner[]
