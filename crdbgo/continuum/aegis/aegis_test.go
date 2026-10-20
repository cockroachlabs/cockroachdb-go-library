package aegis

import (
	"errors"
	"testing"
)

func TestNew_SelectsSimulatedWhenNoEndpoint(t *testing.T) {
	if _, ok := New(Config{}).(*SimulatedClient); !ok {
		t.Error("New(zero Config) should return a *SimulatedClient")
	}
	if _, ok := New(Config{Enabled: true}).(*SimulatedClient); !ok {
		t.Error("New with enabled but no endpoint should return a *SimulatedClient")
	}
}

func TestNew_SelectsHTTPWhenEndpointSet(t *testing.T) {
	c := New(Config{Enabled: true, Endpoint: "https://aegis.example/api", Token: "tok"})
	if _, ok := c.(*HTTPClient); !ok {
		t.Fatalf("New with endpoint should return a *HTTPClient, got %T", c)
	}
	if _, err := c.Findings(); !errors.Is(err, ErrAegisNotPublic) {
		t.Errorf("HTTP Findings() error = %v, want ErrAegisNotPublic", err)
	}
	if _, err := c.Recommendations(); !errors.Is(err, ErrAegisNotPublic) {
		t.Errorf("HTTP Recommendations() error = %v, want ErrAegisNotPublic", err)
	}
	if err := c.Emit(AppEvent{Kind: "x"}); !errors.Is(err, ErrAegisNotPublic) {
		t.Errorf("HTTP Emit() error = %v, want ErrAegisNotPublic", err)
	}
}

func TestSimulated_FindingsAndRecommendations(t *testing.T) {
	c := NewSimulated()
	f, err := c.Findings()
	if err != nil || len(f) == 0 {
		t.Fatalf("Findings() = %v, %v; want non-empty", f, err)
	}
	r, err := c.Recommendations()
	if err != nil || len(r) == 0 {
		t.Fatalf("Recommendations() = %v, %v; want non-empty", r, err)
	}
}

func TestSimulated_EmitRecorded(t *testing.T) {
	c := NewSimulated()
	if err := c.Emit(AppEvent{Kind: "retry", Source: "svc", Attrs: map[string]any{"attempt": 2}, EpochMillis: 123}); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	got := c.EmittedEvents()
	if len(got) != 1 {
		t.Fatalf("EmittedEvents len = %d, want 1", len(got))
	}
	if got[0].Kind != "retry" || got[0].EpochMillis != 123 {
		t.Errorf("event = %+v, want kind=retry epochMillis=123", got[0])
	}
}

func TestExhaust_RecordsRetryAndCircuitOpen(t *testing.T) {
	c := NewSimulated()
	// Deterministic clock.
	ex := NewExhaust(c).WithClock(func() int64 { return 42 })

	if err := ex.RecordRetry("PayrollService.runPayroll", 3); err != nil {
		t.Fatalf("RecordRetry failed: %v", err)
	}
	if err := ex.RecordCircuitOpen("PayrollService.runPayroll"); err != nil {
		t.Fatalf("RecordCircuitOpen failed: %v", err)
	}

	got := c.EmittedEvents()
	if len(got) != 2 {
		t.Fatalf("EmittedEvents len = %d, want 2", len(got))
	}
	if got[0].Kind != "retry" || got[1].Kind != "circuit_open" {
		t.Errorf("kinds = [%q,%q], want [retry, circuit_open]", got[0].Kind, got[1].Kind)
	}
	if got[0].Source != "PayrollService.runPayroll" {
		t.Errorf("source = %q, want PayrollService.runPayroll", got[0].Source)
	}
	if got[0].EpochMillis != 42 || got[1].EpochMillis != 42 {
		t.Errorf("epochMillis = [%d,%d], want injected 42", got[0].EpochMillis, got[1].EpochMillis)
	}
	if got[0].Attrs["attempt"] != 3 {
		t.Errorf("retry attempt attr = %v, want 3", got[0].Attrs["attempt"])
	}
}
