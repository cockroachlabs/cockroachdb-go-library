package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/cdc"
)

// tag::cdc-test[]

// TestIdeaChangefeedHandler_ProjectsEvents feeds a synthetic changefeed batch
// (one insert, one update, one delete) through the handler and asserts it projects
// insert/update after-images onto Ideas and counts the delete. No database.
func TestIdeaChangefeedHandler_ProjectsEvents(t *testing.T) {
	env := &cdc.Envelope{Payload: []cdc.Payload{
		{Table: "ideas", After: map[string]any{"id": "idea-1", "text": "Dark mode", "votes": float64(3)}},
		{Table: "ideas", After: map[string]any{"id": "idea-1", "text": "Dark mode", "votes": float64(4)},
			Before: map[string]any{"id": "idea-1", "text": "Dark mode", "votes": float64(3)}},
		{Table: "ideas", Before: map[string]any{"id": "idea-2", "text": "Gone", "votes": float64(1)}},
	}}

	h := &IdeaChangefeedHandler{}
	if err := h.Handle(context.Background(), cdc.EventsFromEnvelope(env)); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if got := h.Handled(); len(got) != 2 {
		t.Fatalf("handled %d ideas, want 2: %+v", len(got), got)
	}
	if h.Handled()[1].Votes != 4 {
		t.Fatalf("second handled idea votes = %d, want 4", h.Handled()[1].Votes)
	}
	if h.Deletes() != 1 {
		t.Fatalf("deletes = %d, want 1", h.Deletes())
	}
}

// TestIdeaChangefeedHandler_OverWebhook drives the handler through the library's
// cdc.WebhookHandler via httptest, exactly as CockroachDB's webhook sink would —
// a POST of a JSON envelope — proving the wiring end to end without a cluster.
func TestIdeaChangefeedHandler_OverWebhook(t *testing.T) {
	h := &IdeaChangefeedHandler{}
	srv := httptest.NewServer(cdc.NewWebhookHandler(h.Handle))
	defer srv.Close()

	body, _ := json.Marshal(cdc.Envelope{Payload: []cdc.Payload{
		{Table: "ideas", After: map[string]any{"id": "idea-9", "text": "Webhook idea", "votes": float64(7)}},
	}})

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	got := h.Handled()
	if len(got) != 1 || got[0].ID != "idea-9" || got[0].Votes != 7 {
		t.Fatalf("handled = %+v, want one idea-9 with 7 votes", got)
	}
}

// end::cdc-test[]
