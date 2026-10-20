// Package metrics provides [crdbgo.MetricsRecorder] implementations for
// CockroachDB operation tracking.
//
// Two implementations are provided:
//   - [InMemoryRecorder]: thread-safe atomic counters for testing and development.
//   - [OTelRecorder]: OpenTelemetry-backed recorder for production monitoring.
//     Metric names match the Java sibling (cockroachdb-spring-boot-starter)
//     for cross-language dashboard compatibility.
package metrics
