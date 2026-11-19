package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/cdc"
)

// tag::cdc-handler[]

// IdeaChangefeedHandler consumes CockroachDB changefeed events for the ideas
// table. The changefeed webhook delivers batches of decoded cdc.Events; this
// handler turns each insert/update into an Idea and counts deletes, so an app can
// mirror the board into a cache, search index, or audit log as rows change.
//
// Handle has the cdc.EventHandler signature, so it can be passed straight to
// cdc.NewWebhookHandler. It is safe for concurrent delivery.
type IdeaChangefeedHandler struct {
	mu      sync.Mutex
	handled []Idea
	deletes int
}

// Handle processes one webhook batch. Deletes carry no after-image, so they are
// counted; inserts and updates are projected onto an Idea from the after-image.
func (h *IdeaChangefeedHandler) Handle(_ context.Context, events []cdc.Event) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, e := range events {
		if e.Operation == cdc.Delete {
			h.deletes++
			continue
		}
		idea, err := ideaFromAfter(e.After)
		if err != nil {
			return err
		}
		h.handled = append(h.handled, idea)
	}
	return nil
}

// ideaFromAfter projects a changefeed after-image onto an Idea. JSON numbers
// decode as float64, so votes is converted from that.
func ideaFromAfter(after map[string]any) (Idea, error) {
	votes, ok := after["votes"].(float64)
	if !ok {
		return Idea{}, fmt.Errorf("cdc: votes not a number: %v", after["votes"])
	}
	return Idea{
		ID:    fmt.Sprintf("%v", after["id"]),
		Text:  fmt.Sprintf("%v", after["text"]),
		Votes: int(votes),
	}, nil
}

// Handled returns the ideas seen from insert/update events, in arrival order.
func (h *IdeaChangefeedHandler) Handled() []Idea {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Idea(nil), h.handled...)
}

// Deletes returns the number of delete events seen.
func (h *IdeaChangefeedHandler) Deletes() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.deletes
}

// end::cdc-handler[]
