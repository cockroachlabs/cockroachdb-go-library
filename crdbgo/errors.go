package crdbgo

import "errors"

const (
	// SQLStateSerializationFailure is SQLSTATE 40001 — the transaction hit a
	// write-write conflict and must be retried.
	SQLStateSerializationFailure = "40001"

	// SQLStateCockroachRetry is CockroachDB's extension code for retryable errors
	// that are not standard serialization failures.
	SQLStateCockroachRetry = "CR000"
)

// sqlStateError is satisfied by error types that expose a SQL state code.
// pgx's pgconn.PgError does NOT satisfy this (it uses a Code field, not a method),
// so driver-specific adapters provide their own IsRetryable.
type sqlStateError interface {
	SQLState() string
}

// IsRetryableCode reports whether the given SQLSTATE code indicates a
// retryable CockroachDB error.
func IsRetryableCode(code string) bool {
	return code == SQLStateSerializationFailure || code == SQLStateCockroachRetry
}

// IsRetryable checks whether err wraps a retryable CockroachDB error.
// It uses the generic sqlStateError interface. For driver-specific detection
// (e.g., pgx's PgError), use the adapter's IsRetryable instead.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var se sqlStateError
	if errors.As(err, &se) {
		return IsRetryableCode(se.SQLState())
	}
	return false
}

// IsSerializationFailure checks specifically for SQLSTATE 40001.
func IsSerializationFailure(err error) bool {
	if err == nil {
		return false
	}
	var se sqlStateError
	if errors.As(err, &se) {
		return se.SQLState() == SQLStateSerializationFailure
	}
	return false
}
