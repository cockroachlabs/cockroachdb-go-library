package pgxadapter

import (
	"errors"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo"
	"github.com/jackc/pgx/v5/pgconn"
)

// IsRetryable reports whether err represents a retryable CockroachDB error.
// It unwraps pgx's PgError to check the Code field directly, since PgError
// uses a struct field rather than the SQLState() method expected by crdbgo.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return crdbgo.IsRetryableCode(pgErr.Code)
	}
	return crdbgo.IsRetryable(err)
}
