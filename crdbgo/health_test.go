package crdbgo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockHealthChecker implements HealthChecker for testing.
type mockHealthChecker struct {
	err error
}

func (m *mockHealthChecker) Check(_ context.Context) error {
	return m.err
}

func TestHealthHandler_Up(t *testing.T) {
	checker := &mockHealthChecker{err: nil}
	handler := NewHealthHandler(checker)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var status HealthStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if status.Status != "up" {
		t.Errorf("status.Status = %q, want up", status.Status)
	}
	if status.Error != "" {
		t.Errorf("status.Error = %q, want empty", status.Error)
	}
}

func TestHealthHandler_Down(t *testing.T) {
	checker := &mockHealthChecker{err: errors.New("connection refused")}
	handler := NewHealthHandler(checker)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var status HealthStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if status.Status != "down" {
		t.Errorf("status.Status = %q, want down", status.Status)
	}
	if status.Error != "connection refused" {
		t.Errorf("status.Error = %q, want 'connection refused'", status.Error)
	}
}

func TestHealthHandler_ContentType(t *testing.T) {
	checker := &mockHealthChecker{err: nil}
	handler := NewHealthHandler(checker)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}
