package cdc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// EventHandler processes a batch of CDC events from a single webhook delivery.
type EventHandler func(ctx context.Context, events []Event) error

// WebhookHandler is an http.Handler that receives CockroachDB changefeed
// webhook POSTs, parses the envelope, and dispatches events to a handler.
type WebhookHandler struct {
	handler    EventHandler
	onResolved func(ctx context.Context, resolved string)
	maxBody    int64
	metrics    CDCMetrics
}

// WebhookOption configures a WebhookHandler.
type WebhookOption func(*WebhookHandler)

// WithMaxBody sets the maximum request body size in bytes.
// Defaults to 10MB.
func WithMaxBody(maxBytes int64) WebhookOption {
	return func(h *WebhookHandler) { h.maxBody = maxBytes }
}

// WithOnResolved sets a callback for resolved timestamp notifications.
func WithOnResolved(fn func(ctx context.Context, resolved string)) WebhookOption {
	return func(h *WebhookHandler) { h.onResolved = fn }
}

// WithCDCMetrics attaches a CDCMetrics recorder.
func WithCDCMetrics(m CDCMetrics) WebhookOption {
	return func(h *WebhookHandler) { h.metrics = m }
}

// NewWebhookHandler creates an HTTP handler for receiving CockroachDB
// changefeed webhook events.
func NewWebhookHandler(handler EventHandler, opts ...WebhookOption) *WebhookHandler {
	h := &WebhookHandler{
		handler: handler,
		maxBody: 10 * 1024 * 1024, // 10MB default
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// ServeHTTP implements http.Handler.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body := io.LimitReader(r.Body, h.maxBody)
	data, err := io.ReadAll(body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	if h.metrics != nil {
		h.metrics.RecordWebhookReceived(r.URL.Path, int64(len(data)))
	}

	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Handle resolved timestamp notifications.
	if env.IsResolved() {
		if h.metrics != nil {
			h.metrics.RecordResolved(r.URL.Path)
		}
		if h.onResolved != nil {
			h.onResolved(r.Context(), env.Resolved)
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	events := EventsFromEnvelope(&env)
	if err := h.handler(r.Context(), events); err != nil {
		if h.metrics != nil {
			for _, e := range events {
				h.metrics.RecordEvent(r.URL.Path, e.Operation, 0, false)
			}
		}
		http.Error(w, fmt.Sprintf("handler error: %v", err), http.StatusInternalServerError)
		return
	}

	if h.metrics != nil {
		for _, e := range events {
			h.metrics.RecordEvent(r.URL.Path, e.Operation, 0, true)
		}
	}

	w.WriteHeader(http.StatusOK)
}

// WithAuth returns an http.Handler that validates an authentication token
// before delegating to the inner handler. This matches CockroachDB's
// webhook_auth_header changefeed option.
func WithAuth(token string, next http.Handler) http.Handler {
	return WithAuthHeader("Authorization", token, next)
}

// WithAuthHeader returns an http.Handler that validates a custom auth header
// before delegating to the inner handler.
func WithAuthHeader(header, token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get(header)
		if got != token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
