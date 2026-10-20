package cdc

import (
	"strings"
	"testing"
)

func TestChangefeedSpec_Basic(t *testing.T) {
	spec, err := NewChangefeedSpec("orders").
		WebhookURL("https://myapp.com/cdc/orders").
		WithFormat("json").
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sql := spec.ToSQL()
	if !strings.Contains(sql, "CREATE CHANGEFEED FOR TABLE orders") {
		t.Errorf("expected CREATE CHANGEFEED, got %s", sql)
	}
	if !strings.Contains(sql, "webhook-https://myapp.com/cdc/orders") {
		t.Errorf("expected webhook URL in SQL, got %s", sql)
	}
	if !strings.Contains(sql, "format = 'json'") {
		t.Errorf("expected format option, got %s", sql)
	}
}

func TestChangefeedSpec_WithColumns(t *testing.T) {
	spec, err := NewChangefeedSpec("orders").
		Columns("id", "name", "total").
		WebhookURL("https://myapp.com/cdc").
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sql := spec.ToSQL()
	if !strings.Contains(sql, "(id, name, total)") {
		t.Errorf("expected column list, got %s", sql)
	}
}

func TestChangefeedSpec_AllOptions(t *testing.T) {
	spec, err := NewChangefeedSpec("products").
		WebhookURL("https://myapp.com/cdc/products").
		WithFormat("json").
		Updated().
		Resolved("10s").
		Diff().
		InitialScan().
		SchemaChangePolicy("backfill").
		SkipTLSVerify().
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sql := spec.ToSQL()
	if !strings.Contains(sql, "WITH ") {
		t.Error("expected WITH clause")
	}
	// Verify key options are present (order may vary in map iteration).
	for _, opt := range []string{"format", "updated", "resolved", "diff", "initial_scan", "schema_change_policy", "insecure_tls_skip_verify"} {
		if !strings.Contains(sql, opt) {
			t.Errorf("expected option %q in SQL: %s", opt, sql)
		}
	}
}

func TestChangefeedSpec_CustomOption(t *testing.T) {
	spec, err := NewChangefeedSpec("events").
		WebhookURL("https://myapp.com/cdc/events").
		Option("min_checkpoint_frequency", "30s").
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sql := spec.ToSQL()
	if !strings.Contains(sql, "min_checkpoint_frequency = '30s'") {
		t.Errorf("expected custom option, got %s", sql)
	}
}

func TestChangefeedSpec_Table(t *testing.T) {
	spec, _ := NewChangefeedSpec("orders").
		WebhookURL("https://x.com").
		Build()
	if spec.Table() != "orders" {
		t.Errorf("expected table=orders, got %s", spec.Table())
	}
}

func TestChangefeedSpec_WebhookTarget(t *testing.T) {
	spec, _ := NewChangefeedSpec("orders").
		WebhookURL("https://example.com/hook").
		Build()
	if spec.WebhookTarget() != "https://example.com/hook" {
		t.Errorf("expected webhook target, got %s", spec.WebhookTarget())
	}
}

func TestChangefeedSpec_Options(t *testing.T) {
	spec, _ := NewChangefeedSpec("orders").
		WebhookURL("https://x.com").
		WithFormat("json").
		Updated().
		Build()
	opts := spec.Options()
	if opts["format"] != "json" {
		t.Errorf("expected format=json, got %s", opts["format"])
	}
	if _, ok := opts["updated"]; !ok {
		t.Error("expected updated option")
	}
}

// --- Validation tests ---

func TestChangefeedSpec_MissingTable(t *testing.T) {
	_, err := NewChangefeedSpec("").
		WebhookURL("https://x.com").
		Build()
	if err == nil {
		t.Error("expected error for empty table name")
	}
}

func TestChangefeedSpec_InvalidTable(t *testing.T) {
	_, err := NewChangefeedSpec("drop table;--").
		WebhookURL("https://x.com").
		Build()
	if err == nil {
		t.Error("expected error for invalid table name")
	}
}

func TestChangefeedSpec_InvalidColumn(t *testing.T) {
	_, err := NewChangefeedSpec("orders").
		Columns("id", "'; DROP TABLE orders;--").
		WebhookURL("https://x.com").
		Build()
	if err == nil {
		t.Error("expected error for invalid column name")
	}
}

func TestChangefeedSpec_MissingWebhookURL(t *testing.T) {
	_, err := NewChangefeedSpec("orders").Build()
	if err == nil {
		t.Error("expected error for missing webhook URL")
	}
}

func TestChangefeedSpec_NoInitialScan(t *testing.T) {
	spec, err := NewChangefeedSpec("orders").
		WebhookURL("https://x.com").
		NoInitialScan().
		Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sql := spec.ToSQL()
	if !strings.Contains(sql, "no_initial_scan") {
		t.Errorf("expected no_initial_scan option, got %s", sql)
	}
}
