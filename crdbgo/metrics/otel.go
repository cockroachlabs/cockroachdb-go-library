package metrics

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// OTelRecorder implements crdbgo.MetricsRecorder using OpenTelemetry metrics.
// Meter names match the Java sibling (cockroachdb-spring-boot-starter) for
// cross-language dashboard compatibility.
type OTelRecorder struct {
	transactionTotal   metric.Int64Counter
	transactionSuccess metric.Int64Counter
	transactionFailure metric.Int64Counter
	transactionRetries metric.Int64Counter
	transactionDuration metric.Float64Histogram
	followerReadTotal  metric.Int64Counter
	followerReadDuration metric.Float64Histogram
	retryDelay         metric.Float64Histogram
}

// NewOTelRecorder creates a MetricsRecorder backed by an OpenTelemetry Meter.
// The meter should be obtained from your application's MeterProvider.
//
// Example:
//
//	provider := otel.GetMeterProvider()
//	meter := provider.Meter("cockroachdb-go-library")
//	recorder, err := metrics.NewOTelRecorder(meter)
func NewOTelRecorder(meter metric.Meter) (*OTelRecorder, error) {
	r := &OTelRecorder{}
	var err error

	r.transactionTotal, err = meter.Int64Counter("cockroachdb.transactions.total",
		metric.WithDescription("Total number of CockroachDB transactions attempted"),
		metric.WithUnit("{transaction}"),
	)
	if err != nil {
		return nil, err
	}

	r.transactionSuccess, err = meter.Int64Counter("cockroachdb.transactions.success",
		metric.WithDescription("Number of successful CockroachDB transactions"),
		metric.WithUnit("{transaction}"),
	)
	if err != nil {
		return nil, err
	}

	r.transactionFailure, err = meter.Int64Counter("cockroachdb.transactions.failure",
		metric.WithDescription("Number of failed CockroachDB transactions"),
		metric.WithUnit("{transaction}"),
	)
	if err != nil {
		return nil, err
	}

	r.transactionRetries, err = meter.Int64Counter("cockroachdb.transactions.retries",
		metric.WithDescription("Total number of transaction retry attempts"),
		metric.WithUnit("{retry}"),
	)
	if err != nil {
		return nil, err
	}

	r.transactionDuration, err = meter.Float64Histogram("cockroachdb.transactions.duration",
		metric.WithDescription("Duration of CockroachDB transactions"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	r.followerReadTotal, err = meter.Int64Counter("cockroachdb.follower_reads.total",
		metric.WithDescription("Total number of follower read operations"),
		metric.WithUnit("{read}"),
	)
	if err != nil {
		return nil, err
	}

	r.followerReadDuration, err = meter.Float64Histogram("cockroachdb.follower_reads.duration",
		metric.WithDescription("Duration of follower read operations"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	r.retryDelay, err = meter.Float64Histogram("cockroachdb.retries.delay",
		metric.WithDescription("Backoff delay between retries"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	return r, nil
}

// RecordTransaction records a completed transaction.
func (r *OTelRecorder) RecordTransaction(duration time.Duration, retries int, success bool) {
	ctx := context.Background()
	attrs := metric.WithAttributes(
		attribute.Bool("success", success),
	)

	r.transactionTotal.Add(ctx, 1, attrs)
	if success {
		r.transactionSuccess.Add(ctx, 1)
	} else {
		r.transactionFailure.Add(ctx, 1)
	}
	if retries > 0 {
		r.transactionRetries.Add(ctx, int64(retries))
	}
	r.transactionDuration.Record(ctx, float64(duration.Milliseconds()), attrs)
}

// RecordRetry records a single retry attempt.
func (r *OTelRecorder) RecordRetry(delay time.Duration) {
	ctx := context.Background()
	r.retryDelay.Record(ctx, float64(delay.Milliseconds()))
}

// RecordFollowerRead records a follower read query.
func (r *OTelRecorder) RecordFollowerRead(duration time.Duration) {
	ctx := context.Background()
	r.followerReadTotal.Add(ctx, 1)
	r.followerReadDuration.Record(ctx, float64(duration.Milliseconds()))
}
