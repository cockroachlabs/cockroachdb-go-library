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
		name      string
		staleness time.Duration
		want      string
		wantErr   bool
	}{
		{"seconds", 10 * time.Second, "AS OF SYSTEM TIME '-10s'", false},
		{"minutes", 5 * time.Minute, "AS OF SYSTEM TIME '-5m0s'", false},
		{"millis", 500 * time.Millisecond, "AS OF SYSTEM TIME '-500ms'", false},
		{"zero is rejected", 0, "", true},
		{"negative is rejected", -10 * time.Second, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FollowerReadWithStaleness(tt.staleness)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("FollowerReadWithStaleness(%v) = %q, nil; want error", tt.staleness, got)
				}
				if got != "" {
					t.Errorf("FollowerReadWithStaleness(%v) returned clause %q on error; want empty", tt.staleness, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("FollowerReadWithStaleness(%v) unexpected error: %v", tt.staleness, err)
			}
			if got != tt.want {
				t.Errorf("FollowerReadWithStaleness(%v) = %q, want %q", tt.staleness, got, tt.want)
			}
		})
	}
}

func TestFollowerReadClause(t *testing.T) {
	tests := []struct {
		name      string
		staleness time.Duration
		want      string
		wantErr   bool
	}{
		{"zero uses default", 0, "AS OF SYSTEM TIME follower_read_timestamp()", false},
		{"positive uses staleness", 10 * time.Second, "AS OF SYSTEM TIME '-10s'", false},
		{"negative is rejected", -1 * time.Second, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FollowerReadClause(tt.staleness)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("FollowerReadClause(%v) = %q, nil; want error", tt.staleness, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("FollowerReadClause(%v) unexpected error: %v", tt.staleness, err)
			}
			if got != tt.want {
				t.Errorf("FollowerReadClause(%v) = %q, want %q", tt.staleness, got, tt.want)
			}
		})
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
