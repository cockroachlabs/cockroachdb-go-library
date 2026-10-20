package crdbgo

import (
	"errors"
	"fmt"
	"testing"
)

// mockSQLStateError implements error and sqlStateError for testing.
type mockSQLStateError struct {
	code string
}

func (e *mockSQLStateError) Error() string {
	return fmt.Sprintf("sql error: %s", e.code)
}

func (e *mockSQLStateError) SQLState() string {
	return e.code
}

func TestIsRetryableCode_40001(t *testing.T) {
	if !IsRetryableCode("40001") {
		t.Error("IsRetryableCode(40001) = false, want true")
	}
}

func TestIsRetryableCode_CR000(t *testing.T) {
	if !IsRetryableCode("CR000") {
		t.Error("IsRetryableCode(CR000) = false, want true")
	}
}

func TestIsRetryableCode_NonRetryable(t *testing.T) {
	if IsRetryableCode("42601") {
		t.Error("IsRetryableCode(42601) = true, want false")
	}
}

func TestIsRetryableCode_Empty(t *testing.T) {
	if IsRetryableCode("") {
		t.Error("IsRetryableCode('') = true, want false")
	}
}

func TestIsRetryable_Nil(t *testing.T) {
	if IsRetryable(nil) {
		t.Error("IsRetryable(nil) = true, want false")
	}
}

func TestIsRetryable_SQLStateError_40001(t *testing.T) {
	err := &mockSQLStateError{code: "40001"}
	if !IsRetryable(err) {
		t.Error("IsRetryable(40001) = false, want true")
	}
}

func TestIsRetryable_PlainError(t *testing.T) {
	err := errors.New("connection refused")
	if IsRetryable(err) {
		t.Error("IsRetryable(plain error) = true, want false")
	}
}

func TestIsRetryable_WrappedSQLStateError(t *testing.T) {
	inner := &mockSQLStateError{code: "40001"}
	wrapped := fmt.Errorf("transaction failed: %w", inner)
	if !IsRetryable(wrapped) {
		t.Error("IsRetryable(wrapped 40001) = false, want true")
	}
}

func TestIsSerializationFailure_40001(t *testing.T) {
	err := &mockSQLStateError{code: "40001"}
	if !IsSerializationFailure(err) {
		t.Error("IsSerializationFailure(40001) = false, want true")
	}
}

func TestIsSerializationFailure_CR000(t *testing.T) {
	err := &mockSQLStateError{code: "CR000"}
	if IsSerializationFailure(err) {
		t.Error("IsSerializationFailure(CR000) = true, want false")
	}
}

func TestIsSerializationFailure_Nil(t *testing.T) {
	if IsSerializationFailure(nil) {
		t.Error("IsSerializationFailure(nil) = true, want false")
	}
}

func TestIsSerializationFailure_PlainError(t *testing.T) {
	err := errors.New("timeout")
	if IsSerializationFailure(err) {
		t.Error("IsSerializationFailure(plain error) = true, want false")
	}
}
