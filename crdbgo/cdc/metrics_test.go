package cdc

import (
	"testing"
	"time"
)

func TestInMemoryCDCMetrics_RecordEvent(t *testing.T) {
	m := NewInMemoryCDCMetrics()

	m.RecordEvent("/cdc/orders", Insert, 100*time.Millisecond, true)
	m.RecordEvent("/cdc/orders", Update, 50*time.Millisecond, true)
	m.RecordEvent("/cdc/orders", Delete, 0, false)

	snap := m.Snapshot()
	if snap.EventsProcessed != 2 {
		t.Errorf("expected 2 processed, got %d", snap.EventsProcessed)
	}
	if snap.EventsFailed != 1 {
		t.Errorf("expected 1 failed, got %d", snap.EventsFailed)
	}
}

func TestInMemoryCDCMetrics_RecordWebhookReceived(t *testing.T) {
	m := NewInMemoryCDCMetrics()
	m.RecordWebhookReceived("/cdc/orders", 1024)
	m.RecordWebhookReceived("/cdc/orders", 2048)

	snap := m.Snapshot()
	if snap.WebhooksReceived != 2 {
		t.Errorf("expected 2, got %d", snap.WebhooksReceived)
	}
	if snap.TotalPayloadBytes != 3072 {
		t.Errorf("expected 3072 bytes, got %d", snap.TotalPayloadBytes)
	}
}

func TestInMemoryCDCMetrics_RecordResolved(t *testing.T) {
	m := NewInMemoryCDCMetrics()
	m.RecordResolved("/cdc/orders")
	m.RecordResolved("/cdc/orders")

	snap := m.Snapshot()
	if snap.ResolvedCount != 2 {
		t.Errorf("expected 2, got %d", snap.ResolvedCount)
	}
}

func TestInMemoryCDCMetrics_RecordAuthFailure(t *testing.T) {
	m := NewInMemoryCDCMetrics()
	m.RecordAuthFailure("/cdc/orders")

	snap := m.Snapshot()
	if snap.AuthFailures != 1 {
		t.Errorf("expected 1, got %d", snap.AuthFailures)
	}
}

func TestInMemoryCDCMetrics_Reset(t *testing.T) {
	m := NewInMemoryCDCMetrics()
	m.RecordEvent("/cdc/orders", Insert, 0, true)
	m.RecordWebhookReceived("/cdc/orders", 1024)
	m.RecordResolved("/cdc/orders")
	m.RecordAuthFailure("/cdc/orders")

	m.Reset()

	snap := m.Snapshot()
	if snap.EventsProcessed != 0 || snap.WebhooksReceived != 0 || snap.ResolvedCount != 0 || snap.AuthFailures != 0 {
		t.Errorf("expected all zeros after reset, got %+v", snap)
	}
}

func TestInMemoryCDCMetrics_ZeroState(t *testing.T) {
	m := NewInMemoryCDCMetrics()
	snap := m.Snapshot()

	if snap.EventsProcessed != 0 || snap.EventsFailed != 0 || snap.WebhooksReceived != 0 ||
		snap.ResolvedCount != 0 || snap.AuthFailures != 0 || snap.TotalPayloadBytes != 0 {
		t.Errorf("expected all zeros, got %+v", snap)
	}
}

// Verify interface compliance at compile time.
var _ CDCMetrics = (*InMemoryCDCMetrics)(nil)
