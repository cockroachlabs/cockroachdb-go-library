package pgxadapter

import (
	"strings"
	"testing"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsRetryable_InRetryLoop(t *testing.T) {
	policy := crdbgo.DefaultRetryPolicy()

	// Use error interface type so nil assignment produces a true nil interface,
	// not a non-nil interface wrapping a nil *pgconn.PgError pointer.
	var err error = &pgconn.PgError{Code: "40001"}
	retries := 0
	for IsRetryable(err) && policy.ShouldRetry(retries) {
		retries++
		if retries > policy.MaxRetries {
			t.Fatal("exceeded max retries")
		}
		if retries >= 2 {
			err = nil
		}
	}
	if retries != 2 {
		t.Errorf("expected 2 retries, got %d", retries)
	}
}

func TestRetryPolicy_ExhaustedRetries(t *testing.T) {
	policy := crdbgo.RetryPolicy{
		MaxRetries:   3,
		InitialDelay: 0,
		MaxDelay:     0,
		Multiplier:   1,
		Jitter:       false,
	}

	attempts := 0
	for policy.ShouldRetry(attempts) {
		attempts++
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestIsolationLevel_Strings(t *testing.T) {
	tests := []struct {
		level crdbgo.IsolationLevel
		want  string
	}{
		{crdbgo.Serializable, "SERIALIZABLE"},
		{crdbgo.ReadCommitted, "READ COMMITTED"},
	}
	for _, tt := range tests {
		got := tt.level.String()
		if got != tt.want {
			t.Errorf("IsolationLevel(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestFollowerReadTimestamp(t *testing.T) {
	ts := crdbgo.FollowerReadTimestamp()
	if !strings.Contains(ts, "follower_read_timestamp()") {
		t.Errorf("expected follower_read_timestamp() in %q", ts)
	}
	if !strings.HasPrefix(ts, "AS OF SYSTEM TIME") {
		t.Errorf("expected AS OF SYSTEM TIME prefix in %q", ts)
	}
}

func TestFollowerReadQueryConstruction(t *testing.T) {
	query := "SELECT * FROM users WHERE id = $1"
	fullQuery := query + " " + crdbgo.FollowerReadTimestamp()
	if !strings.HasPrefix(fullQuery, "SELECT * FROM users WHERE id = $1") {
		t.Error("original query should be preserved")
	}
	if !strings.HasSuffix(fullQuery, "follower_read_timestamp()") {
		t.Error("should append follower_read_timestamp()")
	}
}

func TestExecuteTxOptions_Defaults(t *testing.T) {
	opts := &ExecuteTxOptions{}
	if opts.ReadOnly != nil {
		t.Error("expected ReadOnly to be nil by default")
	}
	if opts.IsolationLevel != nil {
		t.Error("expected IsolationLevel to be nil by default")
	}
}

func TestExecuteTxOptions_Overrides(t *testing.T) {
	readOnly := true
	iso := "READ COMMITTED"
	opts := &ExecuteTxOptions{
		ReadOnly:       &readOnly,
		IsolationLevel: &iso,
	}
	if *opts.ReadOnly != true {
		t.Error("expected ReadOnly override to be true")
	}
	if *opts.IsolationLevel != "READ COMMITTED" {
		t.Error("expected IsolationLevel override")
	}
}
