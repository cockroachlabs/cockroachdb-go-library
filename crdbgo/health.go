package crdbgo

import (
	"context"
	"encoding/json"
	"net/http"
)

// HealthChecker checks the health of a CockroachDB connection. Each adapter
// (pgxadapter, gormadapter, sqlxadapter) implements this by pinging its
// underlying connection pool.
type HealthChecker interface {
	Check(ctx context.Context) error
}

// HealthStatus represents the health check JSON response body.
type HealthStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// NewHealthHandler returns an HTTP handler that reports connection health.
// Returns 200 {"status":"up"} or 503 {"status":"down","error":"..."}.
// Compatible with any Go HTTP framework (net/http, chi, gin, echo).
func NewHealthHandler(checker HealthChecker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := checker.Check(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(HealthStatus{
				Status: "down",
				Error:  err.Error(),
			})
			return
		}
		json.NewEncoder(w).Encode(HealthStatus{Status: "up"})
	})
}
