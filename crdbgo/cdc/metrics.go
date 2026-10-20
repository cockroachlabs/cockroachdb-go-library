package cdc

import (
	"sync/atomic"
	"time"
)

// CDCMetrics records CDC-specific metrics for webhook event processing,
// resolved timestamps, and authentication failures. Implement this interface
// to export CDC metrics to your monitoring system.
type CDCMetrics interface {
	// RecordEvent records a processed CDC event.
	RecordEvent(endpoint string, operation OperationType, processingNanos time.Duration, success bool)

	// RecordWebhookReceived records an incoming webhook request.
	RecordWebhookReceived(endpoint string, payloadSize int64)

	// RecordResolved records a resolved timestamp notification.
	RecordResolved(endpoint string)

	// RecordAuthFailure records an authentication failure.
	RecordAuthFailure(endpoint string)
}

// InMemoryCDCMetrics is a thread-safe in-memory [CDCMetrics] implementation
// using atomic counters. Useful for testing and development; use a production
// metrics system (e.g., OpenTelemetry) for real deployments.
type InMemoryCDCMetrics struct {
	eventsProcessed   atomic.Int64
	eventsFailed      atomic.Int64
	webhooksReceived  atomic.Int64
	resolvedCount     atomic.Int64
	authFailures      atomic.Int64
	totalPayloadBytes atomic.Int64
	totalProcessingNs atomic.Int64
}

// NewInMemoryCDCMetrics creates a new in-memory CDC metrics recorder.
func NewInMemoryCDCMetrics() *InMemoryCDCMetrics {
	return &InMemoryCDCMetrics{}
}

// RecordEvent records a processed CDC event, incrementing success or failure counters.
func (m *InMemoryCDCMetrics) RecordEvent(_ string, _ OperationType, processingNanos time.Duration, success bool) {
	if success {
		m.eventsProcessed.Add(1)
	} else {
		m.eventsFailed.Add(1)
	}
	m.totalProcessingNs.Add(int64(processingNanos))
}

// RecordWebhookReceived records an incoming webhook request and its payload size.
func (m *InMemoryCDCMetrics) RecordWebhookReceived(_ string, payloadSize int64) {
	m.webhooksReceived.Add(1)
	m.totalPayloadBytes.Add(payloadSize)
}

// RecordResolved records a resolved timestamp notification.
func (m *InMemoryCDCMetrics) RecordResolved(_ string) {
	m.resolvedCount.Add(1)
}

// RecordAuthFailure records an authentication failure on the webhook endpoint.
func (m *InMemoryCDCMetrics) RecordAuthFailure(_ string) {
	m.authFailures.Add(1)
}

// CDCSnapshot holds a point-in-time snapshot of CDC metrics.
type CDCSnapshot struct {
	EventsProcessed   int64
	EventsFailed      int64
	WebhooksReceived  int64
	ResolvedCount     int64
	AuthFailures      int64
	TotalPayloadBytes int64
	TotalProcessingNs int64
}

// Snapshot returns a point-in-time snapshot of the CDC metrics.
func (m *InMemoryCDCMetrics) Snapshot() CDCSnapshot {
	return CDCSnapshot{
		EventsProcessed:   m.eventsProcessed.Load(),
		EventsFailed:      m.eventsFailed.Load(),
		WebhooksReceived:  m.webhooksReceived.Load(),
		ResolvedCount:     m.resolvedCount.Load(),
		AuthFailures:      m.authFailures.Load(),
		TotalPayloadBytes: m.totalPayloadBytes.Load(),
		TotalProcessingNs: m.totalProcessingNs.Load(),
	}
}

// Reset zeroes all counters.
func (m *InMemoryCDCMetrics) Reset() {
	m.eventsProcessed.Store(0)
	m.eventsFailed.Store(0)
	m.webhooksReceived.Store(0)
	m.resolvedCount.Store(0)
	m.authFailures.Store(0)
	m.totalPayloadBytes.Store(0)
	m.totalProcessingNs.Store(0)
}
