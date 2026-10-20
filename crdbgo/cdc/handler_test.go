package cdc

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookHandler_Insert(t *testing.T) {
	var received []Event
	handler := NewWebhookHandler(func(_ context.Context, events []Event) error {
		received = events
		return nil
	})

	env := Envelope{
		Payload: []Payload{
			{After: map[string]any{"id": float64(1), "name": "widget"}, Key: []any{float64(1)}, Table: "products"},
		},
		Length: 1,
	}
	body, _ := json.Marshal(env)

	req := httptest.NewRequest(http.MethodPost, "/cdc/products", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Operation != Insert {
		t.Errorf("expected INSERT, got %s", received[0].Operation)
	}
	if received[0].Table != "products" {
		t.Errorf("expected table=products, got %s", received[0].Table)
	}
}

func TestWebhookHandler_Update(t *testing.T) {
	var received []Event
	handler := NewWebhookHandler(func(_ context.Context, events []Event) error {
		received = events
		return nil
	})

	env := Envelope{
		Payload: []Payload{
			{
				After:  map[string]any{"id": float64(1), "name": "updated"},
				Before: map[string]any{"id": float64(1), "name": "old"},
				Key:    []any{float64(1)},
				Table:  "products",
			},
		},
		Length: 1,
	}
	body, _ := json.Marshal(env)

	req := httptest.NewRequest(http.MethodPost, "/cdc/products", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if received[0].Operation != Update {
		t.Errorf("expected UPDATE, got %s", received[0].Operation)
	}
}

func TestWebhookHandler_Delete(t *testing.T) {
	var received []Event
	handler := NewWebhookHandler(func(_ context.Context, events []Event) error {
		received = events
		return nil
	})

	env := Envelope{
		Payload: []Payload{
			{After: nil, Key: []any{float64(1)}, Table: "products"},
		},
		Length: 1,
	}
	body, _ := json.Marshal(env)

	req := httptest.NewRequest(http.MethodPost, "/cdc/products", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if received[0].Operation != Delete {
		t.Errorf("expected DELETE, got %s", received[0].Operation)
	}
}

func TestWebhookHandler_Resolved(t *testing.T) {
	var resolvedTS string
	handler := NewWebhookHandler(
		func(_ context.Context, events []Event) error {
			t.Error("handler should not be called for resolved")
			return nil
		},
		WithOnResolved(func(_ context.Context, ts string) {
			resolvedTS = ts
		}),
	)

	env := Envelope{Resolved: "1705312200.000000000"}
	body, _ := json.Marshal(env)

	req := httptest.NewRequest(http.MethodPost, "/cdc/products", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if resolvedTS != "1705312200.000000000" {
		t.Errorf("expected resolved timestamp, got %q", resolvedTS)
	}
}

func TestWebhookHandler_MethodNotAllowed(t *testing.T) {
	handler := NewWebhookHandler(func(_ context.Context, events []Event) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/cdc/products", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestWebhookHandler_InvalidJSON(t *testing.T) {
	handler := NewWebhookHandler(func(_ context.Context, events []Event) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/cdc/products", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestWebhookHandler_HandlerError(t *testing.T) {
	handler := NewWebhookHandler(func(_ context.Context, events []Event) error {
		return http.ErrAbortHandler
	})

	env := Envelope{
		Payload: []Payload{
			{After: map[string]any{"id": float64(1)}, Table: "orders"},
		},
	}
	body, _ := json.Marshal(env)

	req := httptest.NewRequest(http.MethodPost, "/cdc/orders", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestWebhookHandler_BatchEvents(t *testing.T) {
	var count int
	handler := NewWebhookHandler(func(_ context.Context, events []Event) error {
		count = len(events)
		return nil
	})

	env := Envelope{
		Payload: []Payload{
			{After: map[string]any{"id": float64(1)}, Table: "orders"},
			{After: map[string]any{"id": float64(2)}, Table: "orders"},
			{After: map[string]any{"id": float64(3)}, Table: "orders"},
		},
		Length: 3,
	}
	body, _ := json.Marshal(env)

	req := httptest.NewRequest(http.MethodPost, "/cdc/orders", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if count != 3 {
		t.Errorf("expected 3 events, got %d", count)
	}
}

// --- Auth middleware tests ---

func TestWithAuth_ValidToken(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := WithAuth("my-secret-token", inner)

	req := httptest.NewRequest(http.MethodPost, "/cdc", nil)
	req.Header.Set("Authorization", "my-secret-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestWithAuth_InvalidToken(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := WithAuth("my-secret-token", inner)

	req := httptest.NewRequest(http.MethodPost, "/cdc", nil)
	req.Header.Set("Authorization", "wrong-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestWithAuth_MissingToken(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := WithAuth("my-secret-token", inner)

	req := httptest.NewRequest(http.MethodPost, "/cdc", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestWithAuthHeader_CustomHeader(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := WithAuthHeader("X-CDC-Auth", "secret123", inner)

	req := httptest.NewRequest(http.MethodPost, "/cdc", nil)
	req.Header.Set("X-CDC-Auth", "secret123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// --- Metrics tests ---

func TestWebhookHandler_WithMetrics(t *testing.T) {
	m := NewInMemoryCDCMetrics()
	handler := NewWebhookHandler(
		func(_ context.Context, events []Event) error { return nil },
		WithCDCMetrics(m),
	)

	env := Envelope{
		Payload: []Payload{
			{After: map[string]any{"id": float64(1)}, Table: "orders"},
		},
	}
	body, _ := json.Marshal(env)

	req := httptest.NewRequest(http.MethodPost, "/cdc/orders", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	snap := m.Snapshot()
	if snap.WebhooksReceived != 1 {
		t.Errorf("expected 1 webhook received, got %d", snap.WebhooksReceived)
	}
	if snap.EventsProcessed != 1 {
		t.Errorf("expected 1 event processed, got %d", snap.EventsProcessed)
	}
}

func TestWebhookHandler_MetricsOnResolved(t *testing.T) {
	m := NewInMemoryCDCMetrics()
	handler := NewWebhookHandler(
		func(_ context.Context, events []Event) error { return nil },
		WithCDCMetrics(m),
	)

	env := Envelope{Resolved: "123456.000"}
	body, _ := json.Marshal(env)

	req := httptest.NewRequest(http.MethodPost, "/cdc/orders", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	snap := m.Snapshot()
	if snap.ResolvedCount != 1 {
		t.Errorf("expected 1 resolved, got %d", snap.ResolvedCount)
	}
}
