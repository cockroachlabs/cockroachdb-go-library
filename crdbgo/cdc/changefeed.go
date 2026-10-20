package cdc

import (
	"fmt"
	"regexp"
	"strings"
)

// identifierRegex validates SQL identifiers to prevent injection.
var identifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// ChangefeedSpec describes a CockroachDB changefeed configuration.
// Use NewChangefeedSpec() to create one with the builder pattern.
type ChangefeedSpec struct {
	table      string
	columns    []string
	webhookURL string
	options    map[string]string
}

// NewChangefeedSpec begins building a changefeed for the given table.
func NewChangefeedSpec(table string) *ChangefeedSpecBuilder {
	return &ChangefeedSpecBuilder{
		spec: ChangefeedSpec{
			table:   table,
			options: make(map[string]string),
		},
	}
}

// ChangefeedSpecBuilder constructs a ChangefeedSpec using the builder pattern.
type ChangefeedSpecBuilder struct {
	spec ChangefeedSpec
	err  error
}

// Columns specifies which columns to include in the changefeed.
// If not called, all columns are included.
func (b *ChangefeedSpecBuilder) Columns(cols ...string) *ChangefeedSpecBuilder {
	for _, col := range cols {
		if !identifierRegex.MatchString(col) {
			b.err = fmt.Errorf("cdc: invalid column name %q", col)
			return b
		}
	}
	b.spec.columns = cols
	return b
}

// WebhookURL sets the destination webhook URL.
func (b *ChangefeedSpecBuilder) WebhookURL(url string) *ChangefeedSpecBuilder {
	b.spec.webhookURL = url
	return b
}

// WithFormat sets the output format (e.g., "json").
func (b *ChangefeedSpecBuilder) WithFormat(format string) *ChangefeedSpecBuilder {
	b.spec.options["format"] = format
	return b
}

// Updated enables the "updated" option to include timestamps on each row.
func (b *ChangefeedSpecBuilder) Updated() *ChangefeedSpecBuilder {
	b.spec.options["updated"] = ""
	return b
}

// Resolved enables resolved timestamp notifications at the given interval.
// Example: "10s", "30s".
func (b *ChangefeedSpecBuilder) Resolved(interval string) *ChangefeedSpecBuilder {
	b.spec.options["resolved"] = interval
	return b
}

// Diff enables the "diff" option to include before-values on updates.
func (b *ChangefeedSpecBuilder) Diff() *ChangefeedSpecBuilder {
	b.spec.options["diff"] = ""
	return b
}

// SchemaChangePolicy sets how schema changes are handled.
// Valid values: "backfill", "nobackfill", "stop".
func (b *ChangefeedSpecBuilder) SchemaChangePolicy(policy string) *ChangefeedSpecBuilder {
	b.spec.options["schema_change_policy"] = policy
	return b
}

// InitialScan enables initial scan of the table when the changefeed starts.
func (b *ChangefeedSpecBuilder) InitialScan() *ChangefeedSpecBuilder {
	b.spec.options["initial_scan"] = ""
	return b
}

// NoInitialScan disables the initial scan.
func (b *ChangefeedSpecBuilder) NoInitialScan() *ChangefeedSpecBuilder {
	b.spec.options["no_initial_scan"] = ""
	return b
}

// SkipTLSVerify disables TLS verification for the webhook endpoint.
// Use with caution — only for development/testing.
func (b *ChangefeedSpecBuilder) SkipTLSVerify() *ChangefeedSpecBuilder {
	b.spec.options["insecure_tls_skip_verify"] = "true"
	return b
}

// Option sets a custom key-value changefeed option.
func (b *ChangefeedSpecBuilder) Option(key, value string) *ChangefeedSpecBuilder {
	b.spec.options[key] = value
	return b
}

// Build validates and returns the ChangefeedSpec.
func (b *ChangefeedSpecBuilder) Build() (*ChangefeedSpec, error) {
	if b.err != nil {
		return nil, b.err
	}
	if b.spec.table == "" {
		return nil, fmt.Errorf("cdc: table name is required")
	}
	if !identifierRegex.MatchString(b.spec.table) {
		return nil, fmt.Errorf("cdc: invalid table name %q", b.spec.table)
	}
	if b.spec.webhookURL == "" {
		return nil, fmt.Errorf("cdc: webhook URL is required")
	}
	return &b.spec, nil
}

// ToSQL generates the CREATE CHANGEFEED SQL statement.
func (s *ChangefeedSpec) ToSQL() string {
	var sb strings.Builder
	sb.WriteString("CREATE CHANGEFEED FOR TABLE ")
	sb.WriteString(s.table)

	if len(s.columns) > 0 {
		sb.WriteString(" (")
		sb.WriteString(strings.Join(s.columns, ", "))
		sb.WriteString(")")
	}

	sb.WriteString(" INTO 'webhook-")
	sb.WriteString(s.webhookURL)
	sb.WriteString("'")

	if len(s.options) > 0 {
		sb.WriteString(" WITH ")
		opts := make([]string, 0, len(s.options))
		for k, v := range s.options {
			if v == "" {
				opts = append(opts, k)
			} else {
				opts = append(opts, fmt.Sprintf("%s = '%s'", k, v))
			}
		}
		sb.WriteString(strings.Join(opts, ", "))
	}

	return sb.String()
}

// Table returns the table name.
func (s *ChangefeedSpec) Table() string { return s.table }

// WebhookTarget returns the webhook URL.
func (s *ChangefeedSpec) WebhookTarget() string { return s.webhookURL }

// Options returns a copy of the changefeed options.
func (s *ChangefeedSpec) Options() map[string]string {
	cp := make(map[string]string, len(s.options))
	for k, v := range s.options {
		cp[k] = v
	}
	return cp
}
