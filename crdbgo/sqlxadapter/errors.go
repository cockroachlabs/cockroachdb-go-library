package sqlxadapter

import (
	"errors"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
	"github.com/jackc/pgx/v5/pgconn"
)

// IsRetryable reports whether err represents a retryable CockroachDB error.
// It checks for pgx's PgError (used by pgx/stdlib under database/sql), then
// falls back to the generic crdbgo.IsRetryable which uses the SQLState() interface.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// pgx/stdlib wraps errors from pgconn, so PgError is reachable.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return crdbgo.IsRetryableCode(pgErr.Code)
	}

	return crdbgo.IsRetryable(err)
}
