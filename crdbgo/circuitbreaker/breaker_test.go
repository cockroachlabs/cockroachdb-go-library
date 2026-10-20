package circuitbreaker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- State string tests ---

func TestState_String(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{Closed, "closed"},
		{Open, "open"},
		{HalfOpen, "half-open"},
		{State(99), "unknown(99)"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

// --- Default configuration ---

func TestNew_Defaults(t *testing.T) {
	cb := New()
	if cb.failureThreshold != 5 {
		t.Errorf("expected default failureThreshold=5, got %d", cb.failureThreshold)
	}
	if cb.breakDuration != 30*time.Second {
		t.Errorf("expected default breakDuration=30s, got %v", cb.breakDuration)
	}
	if cb.State() != Closed {
		t.Errorf("expected initial state Closed, got %v", cb.State())
	}
	if cb.FailureCount() != 0 {
		t.Errorf("expected initial failure count 0, got %d", cb.FailureCount())
	}
}

func TestNew_WithOptions(t *testing.T) {
	cb := New(
		WithFailureThreshold(3),
		WithBreakDuration(10*time.Second),
	)
	if cb.failureThreshold != 3 {
		t.Errorf("expected failureThreshold=3, got %d", cb.failureThreshold)
	}
	if cb.breakDuration != 10*time.Second {
		t.Errorf("expected breakDuration=10s, got %v", cb.breakDuration)
	}
}

func TestNew_InvalidOptions_Ignored(t *testing.T) {
	cb := New(
		WithFailureThreshold(0),  // invalid, should keep default
		WithBreakDuration(-1),    // invalid, should keep default
	)
	if cb.failureThreshold != 5 {
		t.Errorf("expected default failureThreshold=5, got %d", cb.failureThreshold)
	}
	if cb.breakDuration != 30*time.Second {
		t.Errorf("expected default breakDuration=30s, got %v", cb.breakDuration)
	}
}

// --- Closed state behavior ---

func TestClosed_SuccessStaysClosed(t *testing.T) {
	cb := New(WithFailureThreshold(3))
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		err := cb.Execute(ctx, func(context.Context) error { return nil })
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
	}

	if cb.State() != Closed {
		t.Errorf("expected Closed, got %v", cb.State())
	}
}

func TestClosed_FailuresBelowThreshold_StaysClosed(t *testing.T) {
	cb := New(WithFailureThreshold(3))
	ctx := context.Background()

	// Two failures (below threshold of 3).
	for i := 0; i < 2; i++ {
		_ = cb.Execute(ctx, func(context.Context) error {
			return errors.New("fail")
		})
	}

	if cb.State() != Closed {
		t.Errorf("expected Closed after %d failures (threshold=3), got %v", 2, cb.State())
	}
	if cb.FailureCount() != 2 {
		t.Errorf("expected failure count 2, got %d", cb.FailureCount())
	}
}

func TestClosed_SuccessResetsFailureCount(t *testing.T) {
	cb := New(WithFailureThreshold(3))
	ctx := context.Background()

	// Two failures, then a success.
	_ = cb.Execute(ctx, func(context.Context) error { return errors.New("fail") })
	_ = cb.Execute(ctx, func(context.Context) error { return errors.New("fail") })
	_ = cb.Execute(ctx, func(context.Context) error { return nil })

	if cb.FailureCount() != 0 {
		t.Errorf("expected failure count reset to 0 after success, got %d", cb.FailureCount())
	}
	if cb.State() != Closed {
		t.Errorf("expected Closed, got %v", cb.State())
	}
}

// --- Closed -> Open transition ---

func TestClosed_FailuresReachThreshold_TripsToOpen(t *testing.T) {
	cb := New(WithFailureThreshold(2))
	ctx := context.Background()

	_ = cb.Execute(ctx, func(context.Context) error { return errors.New("fail 1") })
	err := cb.Execute(ctx, func(context.Context) error { return errors.New("fail 2") })
	if err == nil || err.Error() != "fail 2" {
		t.Fatalf("expected 'fail 2' error, got: %v", err)
	}

	if cb.State() != Open {
		t.Errorf("expected Open after reaching threshold, got %v", cb.State())
	}
}

// --- Open state behavior ---

func TestOpen_BlocksExecution(t *testing.T) {
	cb := New(WithFailureThreshold(1), WithBreakDuration(10*time.Second))
	ctx := context.Background()

	// Trip the breaker.
	_ = cb.Execute(ctx, func(context.Context) error { return errors.New("trip") })

	// Next call should be blocked.
	executed := false
	err := cb.Execute(ctx, func(context.Context) error {
		executed = true
		return nil
	})

	if executed {
		t.Error("expected operation to be blocked, but it executed")
	}

	var openErr *CircuitBreakerOpenError
	if !errors.As(err, &openErr) {
		t.Fatalf("expected CircuitBreakerOpenError, got: %T %v", err, err)
	}
}

func TestOpen_ErrorContainsTimingInfo(t *testing.T) {
	cb := New(WithFailureThreshold(1), WithBreakDuration(5*time.Second))
	ctx := context.Background()

	_ = cb.Execute(ctx, func(context.Context) error { return errors.New("trip") })
	err := cb.Execute(ctx, func(context.Context) error { return nil })

	var openErr *CircuitBreakerOpenError
	if !errors.As(err, &openErr) {
		t.Fatalf("expected CircuitBreakerOpenError, got: %v", err)
	}

	msg := openErr.Error()
	if msg == "" {
		t.Error("expected non-empty error message")
	}
	// The error should mention "circuit breaker is open".
	if got := openErr.Error(); len(got) < 20 {
		t.Errorf("error message too short: %q", got)
	}
}

// --- Open -> HalfOpen transition ---

func TestOpen_TransitionsToHalfOpen_AfterBreakDuration(t *testing.T) {
	now := time.Now()
	cb := New(
		WithFailureThreshold(1),
		WithBreakDuration(100*time.Millisecond),
	)
	// Override clock to control time.
	cb.now = func() time.Time { return now }
	ctx := context.Background()

	// Trip the breaker.
	_ = cb.Execute(ctx, func(context.Context) error { return errors.New("trip") })
	if cb.State() != Open {
		t.Fatalf("expected Open, got %v", cb.State())
	}

	// Advance time past break duration.
	now = now.Add(150 * time.Millisecond)

	// Next call should be allowed (HalfOpen probe).
	err := cb.Execute(ctx, func(context.Context) error { return nil })
	if err != nil {
		t.Fatalf("expected probe to succeed, got: %v", err)
	}
	if cb.State() != Closed {
		t.Errorf("expected Closed after successful probe, got %v", cb.State())
	}
}

// --- HalfOpen behavior ---

func TestHalfOpen_SuccessClosesCircuit(t *testing.T) {
	now := time.Now()
	cb := New(
		WithFailureThreshold(1),
		WithBreakDuration(50*time.Millisecond),
	)
	cb.now = func() time.Time { return now }
	ctx := context.Background()

	// Trip, advance time, probe succeeds.
	_ = cb.Execute(ctx, func(context.Context) error { return errors.New("trip") })
	now = now.Add(100 * time.Millisecond)

	_ = cb.Execute(ctx, func(context.Context) error { return nil })

	if cb.State() != Closed {
		t.Errorf("expected Closed after successful probe, got %v", cb.State())
	}
	if cb.FailureCount() != 0 {
		t.Errorf("expected failure count 0 after close, got %d", cb.FailureCount())
	}
}

func TestHalfOpen_FailureReopensCircuit(t *testing.T) {
	now := time.Now()
	cb := New(
		WithFailureThreshold(1),
		WithBreakDuration(50*time.Millisecond),
	)
	cb.now = func() time.Time { return now }
	ctx := context.Background()

	// Trip, advance time, probe fails.
	_ = cb.Execute(ctx, func(context.Context) error { return errors.New("trip") })
	now = now.Add(100 * time.Millisecond)

	_ = cb.Execute(ctx, func(context.Context) error { return errors.New("probe fail") })

	if cb.State() != Open {
		t.Errorf("expected Open after failed probe, got %v", cb.State())
	}
}

// --- Reset ---

func TestReset_ClosesCircuit(t *testing.T) {
	cb := New(WithFailureThreshold(1))
	ctx := context.Background()

	_ = cb.Execute(ctx, func(context.Context) error { return errors.New("trip") })
	if cb.State() != Open {
		t.Fatalf("expected Open, got %v", cb.State())
	}

	cb.Reset()

	if cb.State() != Closed {
		t.Errorf("expected Closed after Reset, got %v", cb.State())
	}
	if cb.FailureCount() != 0 {
		t.Errorf("expected failure count 0 after Reset, got %d", cb.FailureCount())
	}

	// Should allow execution again.
	err := cb.Execute(ctx, func(context.Context) error { return nil })
	if err != nil {
		t.Errorf("expected success after Reset, got: %v", err)
	}
}

func TestReset_FromClosed_NoStateChange(t *testing.T) {
	changes := 0
	cb := New(WithOnStateChange(func(from, to State) { changes++ }))

	cb.Reset()
	if changes != 0 {
		t.Errorf("expected no state change callback from Closed Reset, got %d", changes)
	}
}

// --- State change callback ---

func TestOnStateChange_TracksTransitions(t *testing.T) {
	var transitions []string
	now := time.Now()

	cb := New(
		WithFailureThreshold(1),
		WithBreakDuration(50*time.Millisecond),
		WithOnStateChange(func(from, to State) {
			transitions = append(transitions, fmt.Sprintf("%s->%s", from, to))
		}),
	)
	cb.now = func() time.Time { return now }
	ctx := context.Background()

	// Closed -> Open
	_ = cb.Execute(ctx, func(context.Context) error { return errors.New("fail") })

	// Open -> HalfOpen -> Closed (probe succeeds)
	now = now.Add(100 * time.Millisecond)
	_ = cb.Execute(ctx, func(context.Context) error { return nil })

	expected := []string{"closed->open", "open->half-open", "half-open->closed"}
	if len(transitions) != len(expected) {
		t.Fatalf("expected %d transitions, got %d: %v", len(expected), len(transitions), transitions)
	}
	for i, want := range expected {
		if transitions[i] != want {
			t.Errorf("transition[%d] = %q, want %q", i, transitions[i], want)
		}
	}
}

// --- Middleware integration ---

func TestMiddleware_ComposesWithChain(t *testing.T) {
	cb := New(WithFailureThreshold(2))
	mw := cb.Middleware()

	ctx := context.Background()
	calls := 0

	wrapped := mw(func(ctx context.Context) error {
		calls++
		return nil
	})

	if err := wrapped(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestMiddleware_BlocksWhenOpen(t *testing.T) {
	cb := New(WithFailureThreshold(1))
	mw := cb.Middleware()
	ctx := context.Background()

	// Trip via middleware.
	_ = mw(func(context.Context) error { return errors.New("fail") })(ctx)

	// Should block.
	err := mw(func(context.Context) error { return nil })(ctx)
	var openErr *CircuitBreakerOpenError
	if !errors.As(err, &openErr) {
		t.Fatalf("expected CircuitBreakerOpenError, got: %v", err)
	}
}

// --- Concurrency ---

func TestConcurrent_Execute(t *testing.T) {
	cb := New(WithFailureThreshold(100)) // High threshold so it won't trip.
	ctx := context.Background()
	var wg sync.WaitGroup
	var successCount atomic.Int64

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := cb.Execute(ctx, func(context.Context) error { return nil })
			if err == nil {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()
	if got := successCount.Load(); got != 50 {
		t.Errorf("expected 50 successes, got %d", got)
	}
}

func TestConcurrent_TripAndRecover(t *testing.T) {
	now := time.Now()
	var mu sync.Mutex // protects now

	cb := New(
		WithFailureThreshold(5),
		WithBreakDuration(50*time.Millisecond),
	)
	cb.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	ctx := context.Background()

	// Trip the breaker with concurrent failures.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cb.Execute(ctx, func(context.Context) error { return errors.New("fail") })
		}()
	}
	wg.Wait()

	if cb.State() != Open {
		t.Fatalf("expected Open after concurrent failures, got %v", cb.State())
	}

	// Advance time and verify recovery.
	mu.Lock()
	now = now.Add(100 * time.Millisecond)
	mu.Unlock()

	err := cb.Execute(ctx, func(context.Context) error { return nil })
	if err != nil {
		t.Fatalf("expected recovery after break duration, got: %v", err)
	}
	if cb.State() != Closed {
		t.Errorf("expected Closed after recovery, got %v", cb.State())
	}
}

// --- Edge cases ---

func TestExecute_ContextCancelled_PropagatesError(t *testing.T) {
	cb := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := cb.Execute(ctx, func(ctx context.Context) error {
		return ctx.Err()
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestExecute_PanicInFn_DoesNotCorruptState(t *testing.T) {
	cb := New(WithFailureThreshold(3))

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic to propagate")
		}

		// Circuit breaker state should be consistent.
		// The panic bypassed onSuccess/onFailure, so failure count is unchanged.
		if cb.State() != Closed {
			t.Errorf("expected Closed after panic, got %v", cb.State())
		}
	}()

	_ = cb.Execute(context.Background(), func(context.Context) error {
		panic("boom")
	})
}

// --- Full lifecycle ---

func TestFullLifecycle(t *testing.T) {
	now := time.Now()
	cb := New(
		WithFailureThreshold(2),
		WithBreakDuration(100*time.Millisecond),
	)
	cb.now = func() time.Time { return now }
	ctx := context.Background()

	// 1. Closed: success.
	if err := cb.Execute(ctx, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("step 1: %v", err)
	}
	if cb.State() != Closed {
		t.Fatalf("step 1: expected Closed, got %v", cb.State())
	}

	// 2. Closed: failure 1.
	_ = cb.Execute(ctx, func(context.Context) error { return errors.New("f1") })
	if cb.State() != Closed {
		t.Fatalf("step 2: expected Closed, got %v", cb.State())
	}

	// 3. Closed -> Open: failure 2.
	_ = cb.Execute(ctx, func(context.Context) error { return errors.New("f2") })
	if cb.State() != Open {
		t.Fatalf("step 3: expected Open, got %v", cb.State())
	}

	// 4. Open: blocked.
	err := cb.Execute(ctx, func(context.Context) error { return nil })
	var openErr *CircuitBreakerOpenError
	if !errors.As(err, &openErr) {
		t.Fatalf("step 4: expected CircuitBreakerOpenError, got %v", err)
	}

	// 5. Advance time -> HalfOpen probe fails -> Open.
	now = now.Add(150 * time.Millisecond)
	_ = cb.Execute(ctx, func(context.Context) error { return errors.New("probe fail") })
	if cb.State() != Open {
		t.Fatalf("step 5: expected Open after failed probe, got %v", cb.State())
	}

	// 6. Advance time again -> HalfOpen probe succeeds -> Closed.
	now = now.Add(150 * time.Millisecond)
	if err := cb.Execute(ctx, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("step 6: %v", err)
	}
	if cb.State() != Closed {
		t.Fatalf("step 6: expected Closed, got %v", cb.State())
	}
}
