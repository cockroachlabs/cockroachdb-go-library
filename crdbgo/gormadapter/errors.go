package gormadapter

import (
	"errors"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
)

// IsRetryable reports whether a GORM error wraps a retryable CockroachDB error.
// It unwraps through GORM's error chain and checks for serialization failures
// (SQLSTATE 40001) and CockroachDB retry errors (SQLSTATE CR000).
//
// GORM wraps database errors, so this function handles both GORM-wrapped
// errors and raw driver errors.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// First, try the core library's generic check (works with sqlStateError interface).
	if crdbgo.IsRetryable(err) {
		return true
	}

	// Unwrap GORM errors. GORM sometimes wraps errors in its own types.
	var unwrapped error
	unwrapped = errors.Unwrap(err)
	for unwrapped != nil {
		if crdbgo.IsRetryable(unwrapped) {
			return true
		}
		unwrapped = errors.Unwrap(unwrapped)
	}

	return false
}
