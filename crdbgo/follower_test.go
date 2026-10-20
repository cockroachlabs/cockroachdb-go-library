package crdbgo

import (
	"testing"
	"time"
)

func TestFollowerReadTimestamp(t *testing.T) {
	got := FollowerReadTimestamp()
	want := "AS OF SYSTEM TIME follower_read_timestamp()"
	if got != want {
		t.Errorf("FollowerReadTimestamp() = %q, want %q", got, want)
	}
}

func TestFollowerReadWithStaleness(t *testing.T) {
	tests := []struct {
		staleness time.Duration
		want      string
	}{
		{10 * time.Second, "AS OF SYSTEM TIME '-10s'"},
		{5 * time.Minute, "AS OF SYSTEM TIME '-5m0s'"},
		{500 * time.Millisecond, "AS OF SYSTEM TIME '-500ms'"},
	}

	for _, tt := range tests {
		got := FollowerReadWithStaleness(tt.staleness)
		if got != tt.want {
			t.Errorf("FollowerReadWithStaleness(%v) = %q, want %q", tt.staleness, got, tt.want)
		}
	}
}

func TestAsOfSystemTime(t *testing.T) {
	ts := time.Date(2024, 6, 15, 12, 30, 0, 0, time.UTC)
	got := AsOfSystemTime(ts)
	want := "AS OF SYSTEM TIME '2024-06-15T12:30:00Z'"
	if got != want {
		t.Errorf("AsOfSystemTime() = %q, want %q", got, want)
	}
}
