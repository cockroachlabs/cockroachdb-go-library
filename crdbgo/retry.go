package crdbgo

import (
	"math"
	"math/rand/v2"
	"time"
)

// RetryPolicy defines the retry behavior for CockroachDB transactions that
// encounter serialization failures (SQLSTATE 40001) or retryable errors
// (SQLSTATE CR000). It uses exponential backoff with optional jitter to
// prevent thundering herd effects under contention.
type RetryPolicy struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	Jitter       bool
}

// DefaultRetryPolicy returns production-ready retry defaults:
// 5 retries, 50ms initial delay, 5s max delay, 2x multiplier, jitter enabled.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:   5,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
	}
}

// ShouldRetry reports whether another attempt should be made.
func (p RetryPolicy) ShouldRetry(attempt int) bool {
	return attempt < p.MaxRetries
}

// NextDelay computes the backoff delay for the given attempt (0-indexed).
// With jitter enabled, the delay is randomized between 50%-100% of the
// computed value to prevent thundering herd.
func (p RetryPolicy) NextDelay(attempt int) time.Duration {
	delay := float64(p.InitialDelay) * math.Pow(p.Multiplier, float64(attempt))
	if delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}
	if p.Jitter {
		delay = delay * (0.5 + rand.Float64()*0.5)
	}
	return time.Duration(delay)
}
