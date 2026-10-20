package cdc

import (
	"time"
)

// OperationType represents the type of CDC operation.
type OperationType string

const (
	// Insert indicates a new row was inserted.
	Insert OperationType = "INSERT"
	// Update indicates an existing row was modified.
	Update OperationType = "UPDATE"
	// Delete indicates a row was deleted.
	Delete OperationType = "DELETE"
)

// Envelope is the top-level JSON structure sent by CockroachDB's webhook sink.
// Each HTTP POST from the changefeed contains one Envelope.
type Envelope struct {
	Payload  []Payload `json:"payload"`
	Length   int       `json:"length"`
	Resolved string   `json:"resolved,omitempty"`
}

// IsResolved reports whether this envelope is a resolved timestamp
// notification (no data changes, just a checkpoint).
func (e *Envelope) IsResolved() bool {
	return e.Resolved != "" && len(e.Payload) == 0
}

// Payload represents a single row change within a CDC envelope.
// CockroachDB sends after/before as JSON objects (maps).
type Payload struct {
	After   map[string]any `json:"after"`
	Before  map[string]any `json:"before,omitempty"`
	Key     []any          `json:"key"`
	Table   string         `json:"table"`
	Updated string         `json:"updated,omitempty"`
}

// ResolveOperation determines the operation type from the payload fields:
//   - DELETE: after is nil
//   - UPDATE: both after and before are present
//   - INSERT: only after is present
func (p *Payload) ResolveOperation() OperationType {
	if p.After == nil {
		return Delete
	}
	if p.Before != nil {
		return Update
	}
	return Insert
}

// Timestamp parses the updated field into a time.Time.
// Returns the zero time if the field is empty or unparseable.
func (p *Payload) Timestamp() time.Time {
	if p.Updated == "" {
		return time.Time{}
	}
	// CockroachDB sends timestamps in decimal nanosecond format.
	// Try RFC3339 first, then fall back to zero.
	t, err := time.Parse(time.RFC3339Nano, p.Updated)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Event is a processed CDC event with resolved operation type.
// This is the type passed to user handlers.
type Event struct {
	Operation OperationType  `json:"operation"`
	Table     string         `json:"table"`
	After     map[string]any `json:"after,omitempty"`
	Before    map[string]any `json:"before,omitempty"`
	Key       []any          `json:"key"`
	Timestamp time.Time      `json:"timestamp"`
}

// EventsFromEnvelope converts a raw Envelope into processed Events.
func EventsFromEnvelope(env *Envelope) []Event {
	events := make([]Event, 0, len(env.Payload))
	for _, p := range env.Payload {
		events = append(events, Event{
			Operation: p.ResolveOperation(),
			Table:     p.Table,
			After:     p.After,
			Before:    p.Before,
			Key:       p.Key,
			Timestamp: p.Timestamp(),
		})
	}
	return events
}
