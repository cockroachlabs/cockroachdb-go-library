package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/cdc"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/circuitbreaker"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/metrics"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/pgxadapter"
	"github.com/jackc/pgx/v5"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgresql://root@localhost:26257/defaultdb?sslmode=disable"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- Metrics recorder ---
	recorder := metrics.NewInMemoryRecorder()

	// --- Pool with retry config ---
	pool, err := pgxadapter.NewPool(ctx, connStr, []crdbgo.Option{
		crdbgo.WithMaxRetries(5),
		crdbgo.WithJitter(true),
	}, pgxadapter.WithMetrics(recorder))
	if err != nil {
		slog.Error("failed to create pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// --- Circuit breaker ---
	cb := circuitbreaker.New(
		circuitbreaker.WithFailureThreshold(3),
		circuitbreaker.WithBreakDuration(10*time.Second),
		circuitbreaker.WithOnStateChange(func(from, to circuitbreaker.State) {
			slog.Info("circuit breaker state change", "from", from.String(), "to", to.String())
		}),
	)

	// --- Create schema ---
	if err := createSchema(ctx, pool); err != nil {
		slog.Error("failed to create schema", "error", err)
		os.Exit(1)
	}
	slog.Info("schema initialized")

	// --- CDC webhook handler ---
	cdcHandler := cdc.NewWebhookHandler(func(ctx context.Context, events []cdc.Event) error {
		for _, e := range events {
			slog.Info("CDC event received",
				"operation", string(e.Operation),
				"table", e.Table,
				"key", fmt.Sprintf("%v", e.Key),
				"timestamp", e.Timestamp,
			)
		}
		return nil
	})

	// --- Routes ---
	mux := http.NewServeMux()

	mux.Handle("GET /health", crdbgo.NewHealthHandler(pool))
	mux.HandleFunc("GET /metrics", metricsHandler(recorder))
	mux.HandleFunc("POST /tx/insert", insertHandler(ctx, pool))
	mux.HandleFunc("POST /tx/transfer", transferHandler(ctx, pool))
	mux.HandleFunc("GET /follower-read", followerReadHandler(ctx, pool))
	mux.HandleFunc("GET /accounts", listAccountsHandler(ctx, pool))
	mux.Handle("POST /webhook/cdc", cdcHandler)
	mux.HandleFunc("GET /circuit-breaker/status", cbStatusHandler(cb))
	mux.HandleFunc("POST /circuit-breaker/test", cbTestHandler(ctx, cb))

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// --- Startup banner ---
	printBanner(port)

	// --- Graceful shutdown ---
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		slog.Info("shutting down", "signal", sig.String())
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown error", "error", err)
		}
	}()

	slog.Info("server starting", "port", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

func createSchema(ctx context.Context, pool *pgxadapter.Pool) error {
	_, err := pool.RawPool().Exec(ctx, `
		CREATE TABLE IF NOT EXISTS accounts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name TEXT NOT NULL,
			balance DECIMAL NOT NULL DEFAULT 0
		)
	`)
	return err
}

// ---------------------------------------------------------------------------
// JSON helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ---------------------------------------------------------------------------
// GET /metrics
// ---------------------------------------------------------------------------

func metricsHandler(recorder *metrics.InMemoryRecorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := recorder.Snapshot()
		writeJSON(w, http.StatusOK, map[string]any{
			"transaction_count":           snap.TransactionCount,
			"transaction_success_count":   snap.TransactionSuccessCount,
			"transaction_failure_count":   snap.TransactionFailureCount,
			"retry_count":                 snap.RetryCount,
			"follower_read_count":         snap.FollowerReadCount,
			"total_transaction_duration":  snap.TotalTransactionDuration.String(),
			"total_retry_delay":           snap.TotalRetryDelay.String(),
			"total_follower_read_duration": snap.TotalFollowerReadDuration.String(),
		})
	}
}

// ---------------------------------------------------------------------------
// POST /tx/insert
// ---------------------------------------------------------------------------

type insertRequest struct {
	Name    string  `json:"name"`
	Balance float64 `json:"balance"`
}

type insertResponse struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Balance float64 `json:"balance"`
}

func insertHandler(_ context.Context, pool *pgxadapter.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req insertRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}

		var id string
		err := pool.ExecuteTx(r.Context(), func(tx pgx.Tx) error {
			return tx.QueryRow(r.Context(),
				"INSERT INTO accounts (name, balance) VALUES ($1, $2) RETURNING id",
				req.Name, req.Balance,
			).Scan(&id)
		})
		if err != nil {
			slog.Error("insert failed", "error", err)
			writeError(w, http.StatusInternalServerError, "insert failed: "+err.Error())
			return
		}

		slog.Info("account inserted", "id", id, "name", req.Name, "balance", req.Balance)
		writeJSON(w, http.StatusCreated, insertResponse{
			ID:      id,
			Name:    req.Name,
			Balance: req.Balance,
		})
	}
}

// ---------------------------------------------------------------------------
// POST /tx/transfer
// ---------------------------------------------------------------------------

type transferRequest struct {
	From   string  `json:"from"`
	To     string  `json:"to"`
	Amount float64 `json:"amount"`
}

func transferHandler(_ context.Context, pool *pgxadapter.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req transferRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
		if req.From == "" || req.To == "" {
			writeError(w, http.StatusBadRequest, "from and to are required")
			return
		}
		if req.Amount <= 0 {
			writeError(w, http.StatusBadRequest, "amount must be positive")
			return
		}

		err := pool.ExecuteTx(r.Context(), func(tx pgx.Tx) error {
			// Debit sender
			var fromBalance float64
			if err := tx.QueryRow(r.Context(),
				"SELECT balance FROM accounts WHERE id = $1 FOR UPDATE",
				req.From,
			).Scan(&fromBalance); err != nil {
				return fmt.Errorf("sender not found: %w", err)
			}
			if fromBalance < req.Amount {
				return fmt.Errorf("insufficient balance: have %.2f, need %.2f", fromBalance, req.Amount)
			}

			if _, err := tx.Exec(r.Context(),
				"UPDATE accounts SET balance = balance - $1 WHERE id = $2",
				req.Amount, req.From,
			); err != nil {
				return err
			}

			// Credit receiver
			if _, err := tx.Exec(r.Context(),
				"UPDATE accounts SET balance = balance + $1 WHERE id = $2",
				req.Amount, req.To,
			); err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			slog.Error("transfer failed", "error", err)
			writeError(w, http.StatusInternalServerError, "transfer failed: "+err.Error())
			return
		}

		slog.Info("transfer complete", "from", req.From, "to", req.To, "amount", req.Amount)
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "transferred",
			"from":   req.From,
			"to":     req.To,
			"amount": req.Amount,
		})
	}
}

// ---------------------------------------------------------------------------
// GET /follower-read
// ---------------------------------------------------------------------------

type account struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Balance float64 `json:"balance"`
}

func followerReadHandler(_ context.Context, pool *pgxadapter.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := pool.QueryFollowerRead(r.Context(),
			"SELECT id, name, balance FROM accounts",
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "follower read failed: "+err.Error())
			return
		}
		defer rows.Close()

		accounts := make([]account, 0)
		for rows.Next() {
			var a account
			if err := rows.Scan(&a.ID, &a.Name, &a.Balance); err != nil {
				writeError(w, http.StatusInternalServerError, "scan error: "+err.Error())
				return
			}
			accounts = append(accounts, a)
		}
		if err := rows.Err(); err != nil {
			writeError(w, http.StatusInternalServerError, "rows error: "+err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"source":   "follower_read",
			"accounts": accounts,
			"count":    len(accounts),
		})
	}
}

// ---------------------------------------------------------------------------
// GET /accounts
// ---------------------------------------------------------------------------

func listAccountsHandler(_ context.Context, pool *pgxadapter.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := pool.RawPool().Query(r.Context(),
			"SELECT id, name, balance FROM accounts ORDER BY name",
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "query failed: "+err.Error())
			return
		}
		defer rows.Close()

		accounts := make([]account, 0)
		for rows.Next() {
			var a account
			if err := rows.Scan(&a.ID, &a.Name, &a.Balance); err != nil {
				writeError(w, http.StatusInternalServerError, "scan error: "+err.Error())
				return
			}
			accounts = append(accounts, a)
		}
		if err := rows.Err(); err != nil {
			writeError(w, http.StatusInternalServerError, "rows error: "+err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"accounts": accounts,
			"count":    len(accounts),
		})
	}
}

// ---------------------------------------------------------------------------
// GET /circuit-breaker/status
// ---------------------------------------------------------------------------

func cbStatusHandler(cb *circuitbreaker.CircuitBreaker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"state":         cb.State().String(),
			"failure_count": cb.FailureCount(),
		})
	}
}

// ---------------------------------------------------------------------------
// POST /circuit-breaker/test
// ---------------------------------------------------------------------------

func cbTestHandler(_ context.Context, cb *circuitbreaker.CircuitBreaker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := cb.Execute(r.Context(), func(_ context.Context) error {
			return fmt.Errorf("simulated failure for circuit breaker demo")
		})

		status := "failure_recorded"
		httpStatus := http.StatusOK
		errMsg := ""

		if err != nil {
			errMsg = err.Error()
			// Check if the circuit breaker itself blocked the call
			if _, ok := err.(*circuitbreaker.CircuitBreakerOpenError); ok {
				status = "circuit_open"
				httpStatus = http.StatusServiceUnavailable
			}
		}

		writeJSON(w, httpStatus, map[string]any{
			"status":        status,
			"error":         errMsg,
			"breaker_state": cb.State().String(),
			"failure_count": cb.FailureCount(),
		})
	}
}

// ---------------------------------------------------------------------------
// Startup banner
// ---------------------------------------------------------------------------

func printBanner(port string) {
	fmt.Println("==========================================================")
	fmt.Println("  cockroachdb-go-library Demo Server")
	fmt.Println("==========================================================")
	fmt.Println()
	fmt.Println("  Available routes:")
	fmt.Println()
	fmt.Printf("    GET  http://localhost:%s/health              Health check\n", port)
	fmt.Printf("    GET  http://localhost:%s/metrics             Metrics snapshot\n", port)
	fmt.Printf("    POST http://localhost:%s/tx/insert           Insert account (ExecuteTx + retry)\n", port)
	fmt.Printf("    POST http://localhost:%s/tx/transfer         Transfer between accounts\n", port)
	fmt.Printf("    GET  http://localhost:%s/follower-read       Follower read query\n", port)
	fmt.Printf("    GET  http://localhost:%s/accounts            List all accounts\n", port)
	fmt.Printf("    POST http://localhost:%s/webhook/cdc         CDC webhook endpoint\n", port)
	fmt.Printf("    GET  http://localhost:%s/circuit-breaker/status  Circuit breaker state\n", port)
	fmt.Printf("    POST http://localhost:%s/circuit-breaker/test    Trip the circuit breaker\n", port)
	fmt.Println()
	fmt.Println("==========================================================")
}
