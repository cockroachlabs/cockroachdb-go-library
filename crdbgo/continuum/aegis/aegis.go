// Package aegis provides a client for Cockroach Aegis, the autonomous
// database-operations layer of Cockroach Continuum.
//
// The client is read-first: it lists findings and recommendations the
// autonomous-ops agents produce for a cluster, and offers an Emit channel plus
// an [Exhaust] helper so an application can publish structured operational
// telemetry ("what my app is doing / seeing") that Aegis can incorporate.
//
// The public Aegis API is not yet available. Until it is, [New] returns a
// [NewSimulated] in-memory backend whenever no endpoint is configured, so
// applications compile and run whether or not their organization is enrolled.
// [NewHTTP] defines the shape of the real integration but every operation
// currently returns an error rather than inventing a wire protocol.
package aegis

import (
	"errors"
	"sync"
	"time"
)

// ErrAegisNotPublic is returned by the HTTP-backed client while the Cockroach
// Aegis public API is not yet available.
var ErrAegisNotPublic = errors.New("aegis: Aegis API not yet public")

// Finding is something the Cockroach Aegis autonomous-ops agents observed about a
// cluster.
type Finding struct {
	// ID is a stable finding identifier.
	ID string
	// Severity is a severity label (e.g. "info", "warning", "critical").
	Severity string
	// Title is a short summary.
	Title string
	// Detail is a human-readable description.
	Detail string
}

// Recommendation is a suggested action Cockroach Aegis derived from findings.
type Recommendation struct {
	// ID is a stable recommendation identifier.
	ID string
	// Title is a short summary.
	Title string
	// Detail is a human-readable description.
	Detail string
	// SuggestedAction is the concrete action recommended.
	SuggestedAction string
}

// AppEvent is an application-level operational signal emitted to Cockroach Aegis
// (the "exhaust"). The timestamp is supplied explicitly by the caller — never
// read from the wall clock inside this type — so producers stay deterministic and
// testable.
type AppEvent struct {
	// Kind is the event kind (e.g. "retry", "retry_exhausted", "circuit_open").
	Kind string
	// Source is the logical source of the event (e.g. "PayrollService.runPayroll").
	Source string
	// Attrs are arbitrary attributes describing the event.
	Attrs map[string]any
	// EpochMillis is the event time in epoch milliseconds, supplied by the caller.
	EpochMillis int64
}

// Config configures the Cockroach Aegis client.
type Config struct {
	// Enabled reports whether Aegis integration is turned on.
	Enabled bool
	// Endpoint is the Aegis API endpoint. A blank endpoint selects the simulated
	// in-memory backend.
	Endpoint string
	// Token is the bearer token for the Aegis API (may be empty).
	Token string
}

// useSimulated reports whether the simulated backend should be used: Aegis is
// disabled, or no endpoint is configured.
func (c Config) useSimulated() bool {
	return !c.Enabled || c.Endpoint == ""
}

// Client is the read-first Cockroach Aegis surface.
type Client interface {
	// Findings returns the current findings for the cluster.
	Findings() ([]Finding, error)
	// Recommendations returns the current recommendations for the cluster.
	Recommendations() ([]Recommendation, error)
	// Emit publishes an application operational signal to Aegis.
	Emit(event AppEvent) error
}

// New returns a Cockroach Aegis client for cfg. When cfg selects the simulated
// backend (disabled or no endpoint) it returns a [NewSimulated] client;
// otherwise it returns a [NewHTTP] client.
func New(cfg Config) Client {
	if cfg.useSimulated() {
		return NewSimulated()
	}
	return NewHTTP(cfg)
}

// SimulatedClient is an in-memory Cockroach Aegis backend for development and
// tests. It returns a small canned set of findings/recommendations and records
// every emitted [AppEvent] so tests can assert on the exhaust.
type SimulatedClient struct {
	mu              sync.Mutex
	emitted         []AppEvent
	findings        []Finding
	recommendations []Recommendation
}

// NewSimulated creates a [SimulatedClient] seeded with representative findings
// and recommendations.
func NewSimulated() *SimulatedClient {
	return &SimulatedClient{
		findings: []Finding{
			{ID: "f-hotspot-001", Severity: "warning", Title: "Range hotspot detected",
				Detail: "A range is receiving disproportionate traffic relative to its peers."},
			{ID: "f-retry-002", Severity: "info", Title: "Elevated serialization retries",
				Detail: "Serialization retries are above baseline for one workload."},
		},
		recommendations: []Recommendation{
			{ID: "r-split-001", Title: "Split the hot range",
				Detail:          "Splitting the hot range spreads load across more replicas.",
				SuggestedAction: "ALTER TABLE ... SPLIT AT ..."},
			{ID: "r-qos-002", Title: "Lower analytics workload priority",
				Detail:          "Tagging analytics as background QoS reduces contention with OLTP.",
				SuggestedAction: "Set the analytics workload to Low priority."},
		},
	}
}

// Findings returns the canned findings.
func (c *SimulatedClient) Findings() ([]Finding, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Finding, len(c.findings))
	copy(out, c.findings)
	return out, nil
}

// Recommendations returns the canned recommendations.
func (c *SimulatedClient) Recommendations() ([]Recommendation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Recommendation, len(c.recommendations))
	copy(out, c.recommendations)
	return out, nil
}

// Emit records event in memory.
func (c *SimulatedClient) Emit(event AppEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.emitted = append(c.emitted, event)
	return nil
}

// EmittedEvents returns a copy of the events emitted so far, in order.
func (c *SimulatedClient) EmittedEvents() []AppEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]AppEvent, len(c.emitted))
	copy(out, c.emitted)
	return out
}

// compile-time interface check
var _ Client = (*SimulatedClient)(nil)

// HTTPClient is the HTTP-backed Cockroach Aegis client. It defines the shape of
// the real integration; because the Aegis public API is not yet available, every
// operation returns [ErrAegisNotPublic] so callers never silently believe they
// are talking to a live Aegis.
//
// TODO: wire the real Aegis API (findings/recommendations/emit) when it is
// public. Do not invent a wire protocol before then.
type HTTPClient struct {
	config Config
}

// NewHTTP creates an [HTTPClient] for cfg.
func NewHTTP(cfg Config) *HTTPClient {
	return &HTTPClient{config: cfg}
}

// Config returns the configuration this client would use once the Aegis API is
// public.
func (c *HTTPClient) Config() Config { return c.config }

// Findings returns [ErrAegisNotPublic].
func (c *HTTPClient) Findings() ([]Finding, error) { return nil, ErrAegisNotPublic }

// Recommendations returns [ErrAegisNotPublic].
func (c *HTTPClient) Recommendations() ([]Recommendation, error) { return nil, ErrAegisNotPublic }

// Emit returns [ErrAegisNotPublic].
func (c *HTTPClient) Emit(AppEvent) error { return ErrAegisNotPublic }

// compile-time interface check
var _ Client = (*HTTPClient)(nil)

// Clock returns the current time in epoch milliseconds. It is a field on
// [Exhaust] so tests can inject a deterministic clock.
type Clock func() int64

func defaultClock() int64 { return time.Now().UnixMilli() }

// Exhaust wraps a [Client] and emits app-attributed operational signals for
// common resilience events (retries, circuit-breaker trips). Timestamps come
// from an injectable [Clock], so emitted events are deterministic in tests.
type Exhaust struct {
	client Client
	now    Clock
}

// NewExhaust wraps client with a real wall-clock exhaust. Use [Exhaust] with a
// custom Clock via [Exhaust.WithClock] for deterministic tests.
func NewExhaust(client Client) *Exhaust {
	return &Exhaust{client: client, now: defaultClock}
}

// WithClock returns a copy of the exhaust that uses now for event timestamps.
func (e *Exhaust) WithClock(now Clock) *Exhaust {
	return &Exhaust{client: e.client, now: now}
}

// RecordRetry emits a "retry" event attributed to source for the given attempt
// number.
func (e *Exhaust) RecordRetry(source string, attempt int) error {
	return e.client.Emit(AppEvent{
		Kind:        "retry",
		Source:      source,
		Attrs:       map[string]any{"attempt": attempt},
		EpochMillis: e.now(),
	})
}

// RecordCircuitOpen emits a "circuit_open" event attributed to source.
func (e *Exhaust) RecordCircuitOpen(source string) error {
	return e.client.Emit(AppEvent{
		Kind:        "circuit_open",
		Source:      source,
		Attrs:       map[string]any{},
		EpochMillis: e.now(),
	})
}
