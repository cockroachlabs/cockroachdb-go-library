package pgxadapter

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsRetryable_SerializationFailure(t *testing.T) {
	err := &pgconn.PgError{Code: "40001"}
	if !IsRetryable(err) {
		t.Error("expected 40001 to be retryable")
	}
}

func TestIsRetryable_CockroachRetry(t *testing.T) {
	err := &pgconn.PgError{Code: "CR000"}
	if !IsRetryable(err) {
		t.Error("expected CR000 to be retryable")
	}
}

func TestIsRetryable_NonRetryableCode(t *testing.T) {
	err := &pgconn.PgError{Code: "23505"}
	if IsRetryable(err) {
		t.Error("expected 23505 (unique violation) to not be retryable")
	}
}

func TestIsRetryable_WrappedPgError(t *testing.T) {
	inner := &pgconn.PgError{Code: "40001"}
	wrapped := fmt.Errorf("query failed: %w", inner)
	if !IsRetryable(wrapped) {
		t.Error("expected wrapped 40001 to be retryable")
	}
}

func TestIsRetryable_NilError(t *testing.T) {
	if IsRetryable(nil) {
		t.Error("expected nil to not be retryable")
	}
}

func TestIsRetryable_PlainError(t *testing.T) {
	err := errors.New("connection reset")
	if IsRetryable(err) {
		t.Error("expected plain error to not be retryable")
	}
}

func TestIsRetryable_WrappedNonRetryable(t *testing.T) {
	inner := &pgconn.PgError{Code: "42P01"}
	wrapped := fmt.Errorf("table issue: %w", inner)
	if IsRetryable(wrapped) {
		t.Error("expected wrapped 42P01 to not be retryable")
	}
}
