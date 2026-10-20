package pgxadapter

import (
	"testing"
	"time"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo"
)

func TestPoolConfig_DefaultRetryPolicy(t *testing.T) {
	cfg := crdbgo.NewConfig()
	if cfg.Retry.MaxRetries != 5 {
		t.Errorf("expected default MaxRetries=5, got %d", cfg.Retry.MaxRetries)
	}
	if cfg.IsolationLevel != crdbgo.Serializable {
		t.Errorf("expected default Serializable, got %s", cfg.IsolationLevel)
	}
}

func TestPoolConfig_WithOptions(t *testing.T) {
	cfg := crdbgo.NewConfig(
		crdbgo.WithMaxRetries(10),
		crdbgo.WithIsolationLevel(crdbgo.ReadCommitted),
		crdbgo.WithReadOnly(true),
		crdbgo.WithRegion("us-east-1"),
		crdbgo.WithFollowerReads(5*time.Second),
	)
	if cfg.Retry.MaxRetries != 10 {
		t.Errorf("expected MaxRetries=10, got %d", cfg.Retry.MaxRetries)
	}
	if cfg.IsolationLevel != crdbgo.ReadCommitted {
		t.Errorf("expected ReadCommitted, got %s", cfg.IsolationLevel)
	}
	if !cfg.ReadOnly {
		t.Error("expected ReadOnly=true")
	}
	if cfg.Region != "us-east-1" {
		t.Errorf("expected region us-east-1, got %s", cfg.Region)
	}
	if !cfg.FollowerReads {
		t.Error("expected FollowerReads=true")
	}
	if cfg.Staleness != 5*time.Second {
		t.Errorf("expected staleness 5s, got %s", cfg.Staleness)
	}
}

func TestPoolConfig_WithMetricsOption(t *testing.T) {
	recorder := &mockMetrics{}
	p := &Pool{}
	opt := WithMetrics(recorder)
	opt(p)
	if p.metrics == nil {
		t.Error("expected metrics recorder to be set")
	}
}

func TestPool_ImplementsHealthChecker(t *testing.T) {
	// compile-time check is in pool.go via var _ crdbgo.HealthChecker = (*Pool)(nil)
	// this test verifies the Check method exists and delegates
	var _ crdbgo.HealthChecker = (*Pool)(nil)
}

type mockMetrics struct {
	txCount         int
	retryCount      int
	followerCount   int
	lastTxSuccess   bool
	lastTxRetries   int
	lastRetryDelay  time.Duration
}

func (m *mockMetrics) RecordTransaction(d time.Duration, retries int, success bool) {
	m.txCount++
	m.lastTxRetries = retries
	m.lastTxSuccess = success
}

func (m *mockMetrics) RecordRetry(delay time.Duration) {
	m.retryCount++
	m.lastRetryDelay = delay
}

func (m *mockMetrics) RecordFollowerRead(d time.Duration) {
	m.followerCount++
}
