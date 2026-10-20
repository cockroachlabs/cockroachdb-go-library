package crdbtest

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestConcurrencyRunner_Increment(t *testing.T) {
	var counter atomic.Int64
	runner := NewConcurrencyRunner(10, 100)
	result := runner.Run(context.Background(), func(_ context.Context, _ int) error {
		counter.Add(1)
		return nil
	})

	if result.TotalRuns != 1000 {
		t.Errorf("expected 1000 total runs, got %d", result.TotalRuns)
	}
	if result.Successes != 1000 {
		t.Errorf("expected 1000 successes, got %d", result.Successes)
	}
	if result.Failures != 0 {
		t.Errorf("expected 0 failures, got %d", result.Failures)
	}
	if counter.Load() != 1000 {
		t.Errorf("expected counter=1000, got %d", counter.Load())
	}
	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestConcurrencyRunner_ErrorCollection(t *testing.T) {
	sentinel := errors.New("intentional failure")
	runner := NewConcurrencyRunner(4, 5)
	result := runner.Run(context.Background(), func(_ context.Context, _ int) error {
		return sentinel
	})

	if result.Failures != 20 {
		t.Errorf("expected 20 failures, got %d", result.Failures)
	}
	if result.Successes != 0 {
		t.Errorf("expected 0 successes, got %d", result.Successes)
	}
	if len(result.Errors) != 20 {
		t.Errorf("expected 20 collected errors, got %d", len(result.Errors))
	}
}

func TestConcurrencyRunner_MixedResults(t *testing.T) {
	var calls atomic.Int64
	runner := NewConcurrencyRunner(2, 10)
	result := runner.Run(context.Background(), func(_ context.Context, _ int) error {
		n := calls.Add(1)
		if n%3 == 0 {
			return errors.New("every third fails")
		}
		return nil
	})

	if result.TotalRuns != 20 {
		t.Errorf("expected 20 total runs, got %d", result.TotalRuns)
	}
	if result.Successes+result.Failures != 20 {
		t.Errorf("successes + failures should equal 20, got %d", result.Successes+result.Failures)
	}
}

func TestConcurrencyRunner_ZeroWorkers(t *testing.T) {
	runner := NewConcurrencyRunner(0, 10)
	result := runner.Run(context.Background(), func(_ context.Context, _ int) error {
		t.Fatal("should never be called with 0 workers")
		return nil
	})

	if result.TotalRuns != 0 {
		t.Errorf("expected 0 total runs, got %d", result.TotalRuns)
	}
}

func TestConcurrencyRunner_ZeroIterations(t *testing.T) {
	runner := NewConcurrencyRunner(5, 0)
	result := runner.Run(context.Background(), func(_ context.Context, _ int) error {
		t.Fatal("should never be called with 0 iterations")
		return nil
	})

	if result.TotalRuns != 0 {
		t.Errorf("expected 0 total runs, got %d", result.TotalRuns)
	}
}

func TestConcurrencyRunner_WorkerIDsAreDistinct(t *testing.T) {
	seen := make(map[int]bool)
	var mu atomic.Int64
	runner := NewConcurrencyRunner(8, 1)

	var seenMu = struct {
		m map[int]bool
	}{m: seen}
	_ = seenMu

	// Use a channel to collect worker IDs thread-safely.
	ids := make(chan int, 8)
	runner.Run(context.Background(), func(_ context.Context, workerID int) error {
		ids <- workerID
		mu.Add(1)
		return nil
	})
	close(ids)

	collected := make(map[int]bool)
	for id := range ids {
		collected[id] = true
	}
	if len(collected) != 8 {
		t.Errorf("expected 8 distinct worker IDs, got %d", len(collected))
	}
}
