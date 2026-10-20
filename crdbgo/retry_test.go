package crdbgo

import (
	"testing"
	"time"
)

func TestDefaultRetryPolicy_Values(t *testing.T) {
	p := DefaultRetryPolicy()

	if p.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", p.MaxRetries)
	}
	if p.InitialDelay != 50*time.Millisecond {
		t.Errorf("InitialDelay = %v, want 50ms", p.InitialDelay)
	}
	if p.MaxDelay != 5*time.Second {
		t.Errorf("MaxDelay = %v, want 5s", p.MaxDelay)
	}
	if p.Multiplier != 2.0 {
		t.Errorf("Multiplier = %f, want 2.0", p.Multiplier)
	}
	if !p.Jitter {
		t.Error("Jitter = false, want true")
	}
}

func TestShouldRetry_UnderMax(t *testing.T) {
	p := RetryPolicy{MaxRetries: 3}
	for i := 0; i < 3; i++ {
		if !p.ShouldRetry(i) {
			t.Errorf("ShouldRetry(%d) = false, want true (MaxRetries=3)", i)
		}
	}
}

func TestShouldRetry_AtMax(t *testing.T) {
	p := RetryPolicy{MaxRetries: 3}
	if p.ShouldRetry(3) {
		t.Error("ShouldRetry(3) = true, want false (MaxRetries=3)")
	}
}

func TestShouldRetry_OverMax(t *testing.T) {
	p := RetryPolicy{MaxRetries: 3}
	if p.ShouldRetry(5) {
		t.Error("ShouldRetry(5) = true, want false (MaxRetries=3)")
	}
}

func TestNextDelay_NoJitter_Exponential(t *testing.T) {
	p := RetryPolicy{
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       false,
	}

	expected := []time.Duration{
		50 * time.Millisecond,  // 50 * 2^0
		100 * time.Millisecond, // 50 * 2^1
		200 * time.Millisecond, // 50 * 2^2
		400 * time.Millisecond, // 50 * 2^3
	}

	for i, want := range expected {
		got := p.NextDelay(i)
		if got != want {
			t.Errorf("NextDelay(%d) = %v, want %v", i, got, want)
		}
	}
}

func TestNextDelay_CappedByMaxDelay(t *testing.T) {
	p := RetryPolicy{
		InitialDelay: 1 * time.Second,
		MaxDelay:     3 * time.Second,
		Multiplier:   10.0,
		Jitter:       false,
	}

	// attempt 1: 1s * 10^1 = 10s, capped to 3s
	got := p.NextDelay(1)
	if got != 3*time.Second {
		t.Errorf("NextDelay(1) = %v, want 3s (capped)", got)
	}
}

func TestNextDelay_WithJitter_Range(t *testing.T) {
	p := RetryPolicy{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
	}

	// attempt 0: base = 100ms, jitter range = [50ms, 100ms]
	baseDelay := 100 * time.Millisecond
	minExpected := baseDelay / 2
	maxExpected := baseDelay

	for i := 0; i < 100; i++ {
		got := p.NextDelay(0)
		if got < minExpected || got > maxExpected {
			t.Errorf("NextDelay(0) = %v, want in [%v, %v]", got, minExpected, maxExpected)
		}
	}
}
