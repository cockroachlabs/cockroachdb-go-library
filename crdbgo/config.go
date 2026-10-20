package crdbgo

import "time"

// Config holds the complete CockroachDB library settings including retry policy,
// multi-region routing, follower reads, and transaction isolation. Create one
// with [NewConfig] and the With* functional options.
type Config struct {
	Retry          RetryPolicy
	Region         string
	FollowerReads  bool
	Staleness      time.Duration
	IsolationLevel IsolationLevel
	ReadOnly       bool
}

// Option is a functional option that modifies a [Config]. Pass options to
// [NewConfig] to customize behavior.
type Option func(*Config)

// NewConfig creates a Config with production-ready defaults and applies options.
func NewConfig(opts ...Option) *Config {
	cfg := &Config{
		Retry:          DefaultRetryPolicy(),
		IsolationLevel: Serializable,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// WithMaxRetries sets the maximum number of transaction retry attempts.
func WithMaxRetries(n int) Option {
	return func(c *Config) { c.Retry.MaxRetries = n }
}

// WithInitialDelay sets the base delay before the first retry.
func WithInitialDelay(d time.Duration) Option {
	return func(c *Config) { c.Retry.InitialDelay = d }
}

// WithMaxDelay caps the maximum backoff delay between retries.
func WithMaxDelay(d time.Duration) Option {
	return func(c *Config) { c.Retry.MaxDelay = d }
}

// WithMultiplier sets the exponential backoff multiplier.
func WithMultiplier(m float64) Option {
	return func(c *Config) { c.Retry.Multiplier = m }
}

// WithJitter enables or disables random jitter on retry delays.
func WithJitter(enabled bool) Option {
	return func(c *Config) { c.Retry.Jitter = enabled }
}

// WithRegion sets the preferred CockroachDB region for multi-region routing.
func WithRegion(region string) Option {
	return func(c *Config) { c.Region = region }
}

// WithFollowerReads enables follower reads with the specified staleness window.
// Queries will use CockroachDB's AS OF SYSTEM TIME follower_read_timestamp()
// clause, routing reads to the nearest replica rather than the leaseholder.
func WithFollowerReads(staleness time.Duration) Option {
	return func(c *Config) {
		c.FollowerReads = true
		c.Staleness = staleness
	}
}

// WithIsolationLevel sets the transaction isolation level. CockroachDB supports
// Serializable (default, strongest) and ReadCommitted.
func WithIsolationLevel(level IsolationLevel) Option {
	return func(c *Config) { c.IsolationLevel = level }
}

// WithReadOnly marks transactions as read-only, enabling CockroachDB to apply
// optimizations such as skipping write intents and reducing contention.
func WithReadOnly(readOnly bool) Option {
	return func(c *Config) { c.ReadOnly = readOnly }
}
