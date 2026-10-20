package crdbtest

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ConcurrencyResult holds the aggregate outcome of a [ConcurrencyRunner.Run]
// execution: total runs, successes, failures, collected errors, and wall-clock
// duration.
type ConcurrencyResult struct {
	TotalRuns int
	Successes int
	Failures  int
	Retries   int
	Errors    []error
	Duration  time.Duration
}

// ConcurrencyRunner spawns multiple goroutines that execute a function
// concurrently, using a barrier to maximize contention. Useful for testing
// CockroachDB's serialization conflict handling and retry logic under load.
type ConcurrencyRunner struct {
	workers    int
	iterations int
}

// NewConcurrencyRunner creates a runner that will spawn the given number of
// workers, each executing the function for the specified number of iterations.
func NewConcurrencyRunner(workers int, iterations int) *ConcurrencyRunner {
	return &ConcurrencyRunner{workers: workers, iterations: iterations}
}

// Run spawns workers goroutines that each execute fn for iterations times.
// All workers wait at a barrier before starting to maximize contention.
func (r *ConcurrencyRunner) Run(ctx context.Context, fn func(ctx context.Context, workerID int) error) ConcurrencyResult {
	if r.workers <= 0 || r.iterations <= 0 {
		return ConcurrencyResult{}
	}

	var (
		successes atomic.Int64
		failures  atomic.Int64
		mu        sync.Mutex
		errs      []error
		wg        sync.WaitGroup
	)

	// Barrier: all workers block until the channel closes.
	barrier := make(chan struct{})

	wg.Add(r.workers)
	for w := 0; w < r.workers; w++ {
		go func(workerID int) {
			defer wg.Done()
			<-barrier
			for i := 0; i < r.iterations; i++ {
				if err := fn(ctx, workerID); err != nil {
					failures.Add(1)
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
				} else {
					successes.Add(1)
				}
			}
		}(w)
	}

	start := time.Now()
	close(barrier)
	wg.Wait()
	duration := time.Since(start)

	s := int(successes.Load())
	f := int(failures.Load())
	return ConcurrencyResult{
		TotalRuns: s + f,
		Successes: s,
		Failures:  f,
		Errors:    errs,
		Duration:  duration,
	}
}
