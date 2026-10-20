package crdbgo

import "time"

// MetricsRecorder records CockroachDB operation metrics such as transaction
// duration, retry counts, and follower read latency. See the metrics sub-package
// for concrete implementations: [metrics.InMemoryRecorder] for testing and
// [metrics.OTelRecorder] for OpenTelemetry export.
type MetricsRecorder interface {
	// RecordTransaction records a completed transaction with its duration,
	// number of retries, and whether it succeeded.
	RecordTransaction(duration time.Duration, retries int, success bool)

	// RecordRetry records a single retry attempt with its backoff duration.
	RecordRetry(delay time.Duration)

	// RecordFollowerRead records a follower read query with its duration.
	RecordFollowerRead(duration time.Duration)
}
