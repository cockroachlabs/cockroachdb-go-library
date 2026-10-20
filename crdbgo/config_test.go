package crdbgo

import (
	"testing"
	"time"
)

func TestNewConfig_Defaults(t *testing.T) {
	cfg := NewConfig()

	if cfg.Retry.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", cfg.Retry.MaxRetries)
	}
	if cfg.Retry.InitialDelay != 50*time.Millisecond {
		t.Errorf("InitialDelay = %v, want 50ms", cfg.Retry.InitialDelay)
	}
	if cfg.Retry.MaxDelay != 5*time.Second {
		t.Errorf("MaxDelay = %v, want 5s", cfg.Retry.MaxDelay)
	}
	if cfg.Retry.Multiplier != 2.0 {
		t.Errorf("Multiplier = %f, want 2.0", cfg.Retry.Multiplier)
	}
	if !cfg.Retry.Jitter {
		t.Error("Jitter = false, want true")
	}
	if cfg.IsolationLevel != Serializable {
		t.Errorf("IsolationLevel = %v, want Serializable", cfg.IsolationLevel)
	}
	if cfg.FollowerReads {
		t.Error("FollowerReads = true, want false")
	}
	if cfg.ReadOnly {
		t.Error("ReadOnly = true, want false")
	}
	if cfg.Region != "" {
		t.Errorf("Region = %q, want empty", cfg.Region)
	}
}

func TestWithMaxRetries(t *testing.T) {
	cfg := NewConfig(WithMaxRetries(10))
	if cfg.Retry.MaxRetries != 10 {
		t.Errorf("MaxRetries = %d, want 10", cfg.Retry.MaxRetries)
	}
}

func TestWithInitialDelay(t *testing.T) {
	cfg := NewConfig(WithInitialDelay(100 * time.Millisecond))
	if cfg.Retry.InitialDelay != 100*time.Millisecond {
		t.Errorf("InitialDelay = %v, want 100ms", cfg.Retry.InitialDelay)
	}
}

func TestWithMaxDelay(t *testing.T) {
	cfg := NewConfig(WithMaxDelay(10 * time.Second))
	if cfg.Retry.MaxDelay != 10*time.Second {
		t.Errorf("MaxDelay = %v, want 10s", cfg.Retry.MaxDelay)
	}
}

func TestWithMultiplier(t *testing.T) {
	cfg := NewConfig(WithMultiplier(3.0))
	if cfg.Retry.Multiplier != 3.0 {
		t.Errorf("Multiplier = %f, want 3.0", cfg.Retry.Multiplier)
	}
}

func TestWithJitter(t *testing.T) {
	cfg := NewConfig(WithJitter(false))
	if cfg.Retry.Jitter {
		t.Error("Jitter = true, want false")
	}
}

func TestWithRegion(t *testing.T) {
	cfg := NewConfig(WithRegion("us-east-1"))
	if cfg.Region != "us-east-1" {
		t.Errorf("Region = %q, want us-east-1", cfg.Region)
	}
}

func TestWithIsolationLevel(t *testing.T) {
	cfg := NewConfig(WithIsolationLevel(ReadCommitted))
	if cfg.IsolationLevel != ReadCommitted {
		t.Errorf("IsolationLevel = %v, want ReadCommitted", cfg.IsolationLevel)
	}
}

func TestWithReadOnly(t *testing.T) {
	cfg := NewConfig(WithReadOnly(true))
	if !cfg.ReadOnly {
		t.Error("ReadOnly = false, want true")
	}
}

func TestWithFollowerReads(t *testing.T) {
	cfg := NewConfig(WithFollowerReads(10 * time.Second))
	if !cfg.FollowerReads {
		t.Error("FollowerReads = false, want true")
	}
	if cfg.Staleness != 10*time.Second {
		t.Errorf("Staleness = %v, want 10s", cfg.Staleness)
	}
}

func TestMultipleOptionsCompose(t *testing.T) {
	cfg := NewConfig(
		WithMaxRetries(3),
		WithRegion("eu-west-1"),
		WithFollowerReads(5*time.Second),
		WithIsolationLevel(ReadCommitted),
		WithReadOnly(true),
	)

	if cfg.Retry.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.Retry.MaxRetries)
	}
	if cfg.Region != "eu-west-1" {
		t.Errorf("Region = %q, want eu-west-1", cfg.Region)
	}
	if !cfg.FollowerReads {
		t.Error("FollowerReads = false, want true")
	}
	if cfg.Staleness != 5*time.Second {
		t.Errorf("Staleness = %v, want 5s", cfg.Staleness)
	}
	if cfg.IsolationLevel != ReadCommitted {
		t.Errorf("IsolationLevel = %v, want ReadCommitted", cfg.IsolationLevel)
	}
	if !cfg.ReadOnly {
		t.Error("ReadOnly = false, want true")
	}
}
