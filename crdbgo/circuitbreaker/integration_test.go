//go:build integration

package circuitbreaker_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo/circuitbreaker"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/testutil"
)

func TestIntegration_ExecuteWithRealDB(t *testing.T) {
	pool := testutil.PgxPool(t)
	ctx := context.Background()
	table := testutil.UniqueTable("cb_exec")

	// Setup table.
	_, err := pool.Exec(ctx, fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (id INT PRIMARY KEY, val TEXT)", table))
	if err != nil {
		t.Fatalf("create table failed: %v", err)
	}

	cb := circuitbreaker.New(
		circuitbreaker.WithFailureThreshold(3),
		circuitbreaker.WithBreakDuration(1*time.Second),
	)

	// Successful operation should work through the circuit breaker.
	err = cb.Execute(ctx, func(ctx context.Context) error {
		_, err := pool.Exec(ctx, fmt.Sprintf(
			"INSERT INTO %s (id, val) VALUES (1, 'via-cb')", table))
		return err
	})
	if err != nil {
		t.Fatalf("Execute with real DB failed: %v", err)
	}

	// Verify data.
	var val string
	err = pool.QueryRow(ctx, fmt.Sprintf(
		"SELECT val FROM %s WHERE id = 1", table)).Scan(&val)
	if err != nil {
		t.Fatalf("read back failed: %v", err)
	}
	if val != "via-cb" {
		t.Errorf("expected 'via-cb', got %q", val)
	}

	// Verify circuit is still closed.
	if cb.State() != circuitbreaker.Closed {
		t.Errorf("expected Closed state, got %s", cb.State())
	}
}

func TestIntegration_CircuitOpensAfterFailures(t *testing.T) {
	_ = testutil.ConnString(t) // ensure container is running
	ctx := context.Background()

	cb := circuitbreaker.New(
		circuitbreaker.WithFailureThreshold(3),
		circuitbreaker.WithBreakDuration(100*time.Millisecond),
	)

	deliberateErr := errors.New("deliberate failure")

	// Cause 3 consecutive failures to trip the breaker.
	for i := 0; i < 3; i++ {
		err := cb.Execute(ctx, func(_ context.Context) error {
			return deliberateErr
		})
		if !errors.Is(err, deliberateErr) {
			t.Fatalf("iteration %d: expected deliberate error, got %v", i, err)
		}
	}

	// Circuit should now be open.
	if cb.State() != circuitbreaker.Open {
		t.Fatalf("expected Open state after 3 failures, got %s", cb.State())
	}

	// Subsequent calls should be blocked with CircuitBreakerOpenError.
	err := cb.Execute(ctx, func(_ context.Context) error {
		t.Error("function should not be called when circuit is open")
		return nil
	})
	if err == nil {
		t.Fatal("expected error when circuit is open")
	}
	var openErr *circuitbreaker.CircuitBreakerOpenError
	if !errors.As(err, &openErr) {
		t.Fatalf("expected CircuitBreakerOpenError, got %T: %v", err, err)
	}

	// Wait for break duration to elapse, then succeed to close the circuit.
	time.Sleep(150 * time.Millisecond)

	err = cb.Execute(ctx, func(_ context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected success after break duration, got: %v", err)
	}

	// Circuit should be back to closed.
	if cb.State() != circuitbreaker.Closed {
		t.Errorf("expected Closed after successful probe, got %s", cb.State())
	}
}

func TestIntegration_CircuitBreakerWithRealDBFailure(t *testing.T) {
	pool := testutil.PgxPool(t)
	ctx := context.Background()

	cb := circuitbreaker.New(
		circuitbreaker.WithFailureThreshold(2),
		circuitbreaker.WithBreakDuration(200*time.Millisecond),
	)

	// Cause failures by querying a non-existent table.
	for i := 0; i < 2; i++ {
		_ = cb.Execute(ctx, func(ctx context.Context) error {
			_, err := pool.Exec(ctx, "SELECT * FROM nonexistent_table_xyz_123")
			return err
		})
	}

	if cb.State() != circuitbreaker.Open {
		t.Fatalf("expected Open after 2 DB failures, got %s", cb.State())
	}

	// Verify the circuit blocks new operations.
	err := cb.Execute(ctx, func(ctx context.Context) error {
		_, err := pool.Exec(ctx, "SELECT 1")
		return err
	})
	var openErr *circuitbreaker.CircuitBreakerOpenError
	if !errors.As(err, &openErr) {
		t.Fatalf("expected CircuitBreakerOpenError, got: %v", err)
	}

	// Wait and recover.
	time.Sleep(250 * time.Millisecond)

	err = cb.Execute(ctx, func(ctx context.Context) error {
		_, err := pool.Exec(ctx, "SELECT 1")
		return err
	})
	if err != nil {
		t.Fatalf("expected recovery, got: %v", err)
	}
	if cb.State() != circuitbreaker.Closed {
		t.Errorf("expected Closed after recovery, got %s", cb.State())
	}
}
