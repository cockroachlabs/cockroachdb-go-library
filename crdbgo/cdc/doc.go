// Package cdc provides CockroachDB Change Data Capture (CDC) support.
//
// It includes:
//   - A changefeed SQL builder for creating CockroachDB changefeeds
//   - Webhook HTTP handlers for receiving CDC events
//   - Event types matching CockroachDB's webhook payload format
//   - A changefeed manager for lifecycle operations (create, pause, resume, cancel)
//
// Basic usage:
//
//	// Build and execute a changefeed
//	spec := cdc.NewChangefeedSpec("orders").
//	    WebhookURL("https://myapp.com/cdc/orders").
//	    WithFormat("json").
//	    Updated().
//	    Resolved("10s").
//	    Build()
//	sql := spec.ToSQL()
//
//	// Handle webhook events
//	handler := cdc.NewWebhookHandler(func(ctx context.Context, events []cdc.Event) error {
//	    for _, e := range events {
//	        log.Printf("op=%s table=%s", e.Operation, e.Table)
//	    }
//	    return nil
//	})
//	http.Handle("/cdc/orders", cdc.WithAuth("my-token", handler))
package cdc
