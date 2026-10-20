package cdc

import (
	"testing"
)

func TestResolveOperation_Insert(t *testing.T) {
	p := Payload{
		After: map[string]any{"id": 1, "name": "test"},
		Table: "orders",
	}
	if got := p.ResolveOperation(); got != Insert {
		t.Errorf("expected INSERT, got %s", got)
	}
}

func TestResolveOperation_Update(t *testing.T) {
	p := Payload{
		After:  map[string]any{"id": 1, "name": "updated"},
		Before: map[string]any{"id": 1, "name": "old"},
		Table:  "orders",
	}
	if got := p.ResolveOperation(); got != Update {
		t.Errorf("expected UPDATE, got %s", got)
	}
}

func TestResolveOperation_Delete(t *testing.T) {
	p := Payload{
		After: nil,
		Key:   []any{1},
		Table: "orders",
	}
	if got := p.ResolveOperation(); got != Delete {
		t.Errorf("expected DELETE, got %s", got)
	}
}

func TestPayload_Timestamp_Valid(t *testing.T) {
	p := Payload{
		Updated: "2024-01-15T10:30:00Z",
	}
	ts := p.Timestamp()
	if ts.IsZero() {
		t.Error("expected non-zero timestamp")
	}
	if ts.Year() != 2024 || ts.Month() != 1 || ts.Day() != 15 {
		t.Errorf("unexpected timestamp: %v", ts)
	}
}

func TestPayload_Timestamp_Empty(t *testing.T) {
	p := Payload{}
	ts := p.Timestamp()
	if !ts.IsZero() {
		t.Errorf("expected zero timestamp, got %v", ts)
	}
}

func TestPayload_Timestamp_Invalid(t *testing.T) {
	p := Payload{Updated: "not-a-timestamp"}
	ts := p.Timestamp()
	if !ts.IsZero() {
		t.Errorf("expected zero timestamp for invalid input, got %v", ts)
	}
}

func TestEnvelope_IsResolved_True(t *testing.T) {
	env := Envelope{
		Resolved: "1705312200.000000000",
		Payload:  nil,
	}
	if !env.IsResolved() {
		t.Error("expected IsResolved=true")
	}
}

func TestEnvelope_IsResolved_False_HasPayload(t *testing.T) {
	env := Envelope{
		Resolved: "1705312200.000000000",
		Payload:  []Payload{{Table: "orders"}},
	}
	if env.IsResolved() {
		t.Error("expected IsResolved=false when payload is present")
	}
}

func TestEnvelope_IsResolved_False_NoResolved(t *testing.T) {
	env := Envelope{
		Payload: []Payload{{Table: "orders"}},
	}
	if env.IsResolved() {
		t.Error("expected IsResolved=false when resolved is empty")
	}
}

func TestEventsFromEnvelope_MultiplePayloads(t *testing.T) {
	env := &Envelope{
		Payload: []Payload{
			{After: map[string]any{"id": 1}, Table: "orders"},
			{After: map[string]any{"id": 2}, Before: map[string]any{"id": 2}, Table: "orders"},
			{After: nil, Key: []any{3}, Table: "orders"},
		},
		Length: 3,
	}

	events := EventsFromEnvelope(env)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	if events[0].Operation != Insert {
		t.Errorf("event[0]: expected INSERT, got %s", events[0].Operation)
	}
	if events[1].Operation != Update {
		t.Errorf("event[1]: expected UPDATE, got %s", events[1].Operation)
	}
	if events[2].Operation != Delete {
		t.Errorf("event[2]: expected DELETE, got %s", events[2].Operation)
	}
}

func TestEventsFromEnvelope_EmptyPayload(t *testing.T) {
	env := &Envelope{Payload: nil}
	events := EventsFromEnvelope(env)
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestEventsFromEnvelope_PreservesTableAndKey(t *testing.T) {
	env := &Envelope{
		Payload: []Payload{
			{
				After: map[string]any{"id": 42, "name": "widget"},
				Key:   []any{42},
				Table: "products",
			},
		},
	}
	events := EventsFromEnvelope(env)
	if events[0].Table != "products" {
		t.Errorf("expected table=products, got %s", events[0].Table)
	}
	if len(events[0].Key) != 1 {
		t.Fatalf("expected 1 key element, got %d", len(events[0].Key))
	}
}
