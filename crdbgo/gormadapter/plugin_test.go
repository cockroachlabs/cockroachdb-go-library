package gormadapter

import (
	"testing"
	"time"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/metrics"
	"gorm.io/gorm/clause"
)

// --- Plugin tests ---

func TestNewPlugin_Defaults(t *testing.T) {
	p := NewPlugin(nil)
	if p == nil {
		t.Fatal("expected non-nil plugin")
	}
	if p.config == nil {
		t.Fatal("expected non-nil config")
	}
	if p.metrics != nil {
		t.Error("expected nil metrics by default")
	}
}

func TestNewPlugin_WithOptions(t *testing.T) {
	p := NewPlugin([]crdbgo.Option{
		crdbgo.WithMaxRetries(7),
		crdbgo.WithRegion("eu-west-1"),
	})

	if p.config.Retry.MaxRetries != 7 {
		t.Errorf("expected MaxRetries=7, got %d", p.config.Retry.MaxRetries)
	}
	if p.config.Region != "eu-west-1" {
		t.Errorf("expected region eu-west-1, got %s", p.config.Region)
	}
}

func TestNewPlugin_WithMetrics(t *testing.T) {
	rec := metrics.NewInMemoryRecorder()
	p := NewPlugin(nil, WithMetrics(rec))

	if p.metrics == nil {
		t.Fatal("expected non-nil metrics")
	}
}

func TestPlugin_Name(t *testing.T) {
	p := NewPlugin(nil)
	if got := p.Name(); got != "crdbgo:cockroachdb" {
		t.Errorf("expected plugin name 'crdbgo:cockroachdb', got %q", got)
	}
}

func TestNewPlugin_AllOptions(t *testing.T) {
	p := NewPlugin([]crdbgo.Option{
		crdbgo.WithMaxRetries(10),
		crdbgo.WithInitialDelay(200 * time.Millisecond),
		crdbgo.WithMaxDelay(10 * time.Second),
		crdbgo.WithMultiplier(3.0),
		crdbgo.WithJitter(false),
		crdbgo.WithRegion("ap-southeast-1"),
		crdbgo.WithIsolationLevel(crdbgo.ReadCommitted),
		crdbgo.WithReadOnly(true),
	})

	if p.config.Retry.MaxRetries != 10 {
		t.Errorf("expected MaxRetries=10, got %d", p.config.Retry.MaxRetries)
	}
	if p.config.Retry.InitialDelay != 200*time.Millisecond {
		t.Errorf("expected InitialDelay=200ms, got %v", p.config.Retry.InitialDelay)
	}
	if p.config.Retry.MaxDelay != 10*time.Second {
		t.Errorf("expected MaxDelay=10s, got %v", p.config.Retry.MaxDelay)
	}
	if p.config.Retry.Multiplier != 3.0 {
		t.Errorf("expected Multiplier=3.0, got %v", p.config.Retry.Multiplier)
	}
	if p.config.Retry.Jitter {
		t.Error("expected Jitter=false")
	}
	if p.config.Region != "ap-southeast-1" {
		t.Errorf("expected region ap-southeast-1, got %s", p.config.Region)
	}
	if p.config.IsolationLevel != crdbgo.ReadCommitted {
		t.Errorf("expected ReadCommitted isolation, got %v", p.config.IsolationLevel)
	}
	if !p.config.ReadOnly {
		t.Error("expected ReadOnly=true")
	}
}

// --- Error classification tests ---

func TestIsRetryable_Nil(t *testing.T) {
	if IsRetryable(nil) {
		t.Error("expected nil error to not be retryable")
	}
}

func TestIsRetryable_NonRetryableError(t *testing.T) {
	err := &testError{code: "42P01"} // undefined table
	if IsRetryable(err) {
		t.Error("expected non-retryable SQLSTATE to return false")
	}
}

func TestIsRetryable_SerializationFailure(t *testing.T) {
	err := &testError{code: "40001"}
	if !IsRetryable(err) {
		t.Error("expected SQLSTATE 40001 to be retryable")
	}
}

func TestIsRetryable_CockroachRetry(t *testing.T) {
	err := &testError{code: "CR000"}
	if !IsRetryable(err) {
		t.Error("expected SQLSTATE CR000 to be retryable")
	}
}

// --- Scope tests (unit-level, no DB required) ---

func TestFollowerRead_ReturnsScope(t *testing.T) {
	scope := FollowerRead()
	if scope == nil {
		t.Fatal("expected non-nil scope function")
	}
}

func TestFollowerReadWithStaleness_ReturnsScope(t *testing.T) {
	scope := FollowerReadWithStaleness(10 * time.Second)
	if scope == nil {
		t.Fatal("expected non-nil scope function")
	}
}

func TestInRegion_ReturnsScope(t *testing.T) {
	scope := InRegion("us-east-1")
	if scope == nil {
		t.Fatal("expected non-nil scope function")
	}
}

func TestReadCommitted_ReturnsScope(t *testing.T) {
	scope := ReadCommitted()
	if scope == nil {
		t.Fatal("expected non-nil scope function")
	}
}

func TestReadOnly_ReturnsScope(t *testing.T) {
	scope := ReadOnly()
	if scope == nil {
		t.Fatal("expected non-nil scope function")
	}
}

// --- AsOfSystemTimeClause tests ---

func TestAsOfSystemTimeClause_Name(t *testing.T) {
	c := AsOfSystemTimeClause{SQL: "AS OF SYSTEM TIME follower_read_timestamp()"}
	if got := c.Name(); got != "AS OF SYSTEM TIME" {
		t.Errorf("expected clause name 'AS OF SYSTEM TIME', got %q", got)
	}
}

func TestAsOfSystemTimeClause_Build(t *testing.T) {
	c := AsOfSystemTimeClause{SQL: "AS OF SYSTEM TIME follower_read_timestamp()"}
	b := &testBuilder{}
	c.Build(b)
	if b.written != "AS OF SYSTEM TIME follower_read_timestamp()" {
		t.Errorf("expected follower_read_timestamp SQL, got %q", b.written)
	}
}

func TestAsOfSystemTimeClause_BuildStaleness(t *testing.T) {
	c := AsOfSystemTimeClause{SQL: "AS OF SYSTEM TIME '-10s'"}
	b := &testBuilder{}
	c.Build(b)
	if b.written != "AS OF SYSTEM TIME '-10s'" {
		t.Errorf("expected staleness SQL, got %q", b.written)
	}
}

// --- Callback state tests ---

func TestRetryStateFromContext_NilDB(t *testing.T) {
	// This should not panic with a nil statement.
	result := retryStateFromContext(nil)
	if result != nil {
		t.Error("expected nil from nil DB")
	}
}

// --- Helpers ---

// testError implements the sqlStateError interface for testing.
type testError struct {
	code string
}

func (e *testError) Error() string   { return "test error: " + e.code }
func (e *testError) SQLState() string { return e.code }

// testBuilder captures WriteString calls for clause.Builder testing.
type testBuilder struct {
	written string
}

func (b *testBuilder) WriteString(s string) (int, error) {
	b.written += s
	return len(s), nil
}

func (b *testBuilder) WriteByte(c byte) error {
	b.written += string(c)
	return nil
}

func (b *testBuilder) WriteQuoted(field interface{}) {
	// no-op for testing
}

func (b *testBuilder) AddVar(writer clause.Writer, vars ...interface{}) {
	// no-op for testing
}

func (b *testBuilder) AddError(err error) error {
	return err
}
