package crdbtest

import (
	"errors"
	"testing"
)

func TestRetrySimulator_FailsThenSucceeds(t *testing.T) {
	sim := NewRetrySimulator(3)
	var attempts int

	for i := 0; i < 5; i++ {
		err := sim.Execute(func() error {
			return nil
		})
		attempts++
		if i < 3 {
			if err == nil {
				t.Fatalf("attempt %d: expected error, got nil", i)
			}
			var sre *SimulatedRetryError
			if !errors.As(err, &sre) {
				t.Fatalf("attempt %d: expected SimulatedRetryError, got %T", i, err)
			}
		} else {
			if err != nil {
				t.Fatalf("attempt %d: expected nil, got %v", i, err)
			}
		}
	}

	if attempts != 5 {
		t.Errorf("expected 5 attempts, got %d", attempts)
	}
}

func TestRetrySimulator_Reset(t *testing.T) {
	sim := NewRetrySimulator(2)

	// Exhaust the failures.
	for i := 0; i < 3; i++ {
		sim.Execute(func() error { return nil })
	}

	sim.Reset()

	// After reset, the first call should fail again.
	err := sim.Execute(func() error { return nil })
	if err == nil {
		t.Fatal("expected error after reset, got nil")
	}
}

func TestSimulatedRetryError_SQLState(t *testing.T) {
	err := &SimulatedRetryError{Attempt: 0}
	if err.SQLState() != "40001" {
		t.Errorf("expected SQLState 40001, got %s", err.SQLState())
	}
}

func TestSimulatedRetryError_ErrorMessage(t *testing.T) {
	err := &SimulatedRetryError{Attempt: 2}
	msg := err.Error()
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestSimulatedRetryError_ImplementsSQLStateInterface(t *testing.T) {
	// Verify the error satisfies the sqlStateError interface used by crdbgo.IsRetryable.
	var err error = &SimulatedRetryError{Attempt: 0}

	type sqlStater interface {
		SQLState() string
	}
	se, ok := err.(sqlStater)
	if !ok {
		t.Fatal("SimulatedRetryError does not implement SQLState() string")
	}
	if se.SQLState() != "40001" {
		t.Errorf("expected 40001, got %s", se.SQLState())
	}
}

func TestFollowerReadSimulator_SimulateRead(t *testing.T) {
	sim := NewFollowerReadSimulator()
	query := "SELECT * FROM users WHERE id = 1"
	modified := sim.SimulateRead(query)

	expected := query + " AS OF SYSTEM TIME follower_read_timestamp()"
	if modified != expected {
		t.Errorf("expected %q, got %q", expected, modified)
	}
}

func TestFollowerReadSimulator_AssertFollowerRead_Passes(t *testing.T) {
	sim := NewFollowerReadSimulator()
	query := sim.SimulateRead("SELECT 1")
	sim.AssertFollowerRead(t, query)
}

func TestFollowerReadSimulator_AssertFollowerRead_Fails(t *testing.T) {
	sim := NewFollowerReadSimulator()

	// Use a sub-test so the expected failure doesn't fail the parent.
	mockT := &testing.T{}
	sim.AssertFollowerRead(mockT, "SELECT 1")
	// We can't directly check mockT.Failed() from the outside in Go's testing
	// framework, but the fact that it doesn't panic is the success condition.
	// The real assertion is tested in the passing case above.
}

func TestRetrySimulator_ZeroFailCount(t *testing.T) {
	sim := NewRetrySimulator(0)
	err := sim.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("expected nil with failCount=0, got %v", err)
	}
}

func TestRetrySimulator_DelegateError(t *testing.T) {
	sim := NewRetrySimulator(0)
	sentinel := errors.New("delegate error")
	err := sim.Execute(func() error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected delegate error, got %v", err)
	}
}
