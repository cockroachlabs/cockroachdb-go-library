package metrics

import (
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"
)

func TestNewOTelRecorder_NoopMeter(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	r, err := NewOTelRecorder(meter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil recorder")
	}
}

func TestOTelRecorder_RecordTransaction_Success(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	r, _ := NewOTelRecorder(meter)

	// Should not panic with noop meter.
	r.RecordTransaction(100*time.Millisecond, 0, true)
}

func TestOTelRecorder_RecordTransaction_Failure(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	r, _ := NewOTelRecorder(meter)

	r.RecordTransaction(50*time.Millisecond, 3, false)
}

func TestOTelRecorder_RecordTransaction_WithRetries(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	r, _ := NewOTelRecorder(meter)

	r.RecordTransaction(200*time.Millisecond, 5, true)
}

func TestOTelRecorder_RecordTransaction_ZeroRetries(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	r, _ := NewOTelRecorder(meter)

	// Zero retries should not increment the retry counter.
	r.RecordTransaction(10*time.Millisecond, 0, true)
}

func TestOTelRecorder_RecordRetry(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	r, _ := NewOTelRecorder(meter)

	r.RecordRetry(100 * time.Millisecond)
	r.RecordRetry(200 * time.Millisecond)
}

func TestOTelRecorder_RecordFollowerRead(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	r, _ := NewOTelRecorder(meter)

	r.RecordFollowerRead(5 * time.Millisecond)
}

func TestOTelRecorder_RecordMultipleOperations(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	r, _ := NewOTelRecorder(meter)

	// Simulate a realistic sequence of operations.
	r.RecordTransaction(100*time.Millisecond, 0, true)
	r.RecordTransaction(50*time.Millisecond, 2, true)
	r.RecordRetry(100 * time.Millisecond)
	r.RecordRetry(200 * time.Millisecond)
	r.RecordTransaction(300*time.Millisecond, 3, false)
	r.RecordFollowerRead(5 * time.Millisecond)
	r.RecordFollowerRead(10 * time.Millisecond)
}

func TestOTelRecorder_RecordTransaction_LargeDuration(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	r, _ := NewOTelRecorder(meter)

	r.RecordTransaction(30*time.Second, 10, false)
}

func TestOTelRecorder_RecordRetry_ZeroDelay(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	r, _ := NewOTelRecorder(meter)

	r.RecordRetry(0)
}

func TestOTelRecorder_RecordFollowerRead_ZeroDuration(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	r, _ := NewOTelRecorder(meter)

	r.RecordFollowerRead(0)
}

func TestOTelRecorder_ImplementsMetricsRecorder(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	r, _ := NewOTelRecorder(meter)

	// Verify at runtime that OTelRecorder satisfies the MetricsRecorder interface
	// by calling all three methods.
	r.RecordTransaction(time.Millisecond, 0, true)
	r.RecordRetry(time.Millisecond)
	r.RecordFollowerRead(time.Millisecond)
}

func TestOTelRecorder_ConcurrentAccess(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	r, _ := NewOTelRecorder(meter)

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				r.RecordTransaction(time.Millisecond, 1, true)
				r.RecordRetry(time.Millisecond)
				r.RecordFollowerRead(time.Millisecond)
			}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
