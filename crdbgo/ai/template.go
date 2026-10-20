package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// VectorSearchTemplate provides low-level vector search and storage operations
// against CockroachDB's native VECTOR type. It generates and executes SQL directly.
type VectorSearchTemplate struct {
	db      SQLDB
	metrics AIMetrics
}

// NewVectorSearchTemplate creates a template with the given database connection.
func NewVectorSearchTemplate(db SQLDB, opts ...TemplateOption) *VectorSearchTemplate {
	t := &VectorSearchTemplate{
		db:      db,
		metrics: &noopAIMetrics{},
	}
	for _, o := range opts {
		o(t)
	}
	return t
}

// TemplateOption configures a VectorSearchTemplate.
type TemplateOption func(*VectorSearchTemplate)

// WithTemplateMetrics sets the metrics recorder for the template.
func WithTemplateMetrics(m AIMetrics) TemplateOption {
	return func(t *VectorSearchTemplate) {
		if m != nil {
			t.metrics = m
		}
	}
}

// InitSchema creates the vector table if it doesn't exist.
// dims specifies the vector dimensionality (e.g., 1536 for OpenAI ada-002).
//
// Deprecated: Use [VectorSearchTemplate.InitSchemaV26] for CockroachDB v26.2+ which
// supports CREATE VECTOR INDEX with prefix columns and the vector_index feature flag.
func (t *VectorSearchTemplate) InitSchema(ctx context.Context, table, embeddingColumn string, dims int) error {
	if err := validateIdentifier(table); err != nil {
		return fmt.Errorf("ai: invalid table name: %w", err)
	}
	if err := validateIdentifier(embeddingColumn); err != nil {
		return fmt.Errorf("ai: invalid column name: %w", err)
	}
	if dims <= 0 {
		return errors.New("ai: dimensions must be positive")
	}

	query := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			content TEXT,
			metadata JSONB DEFAULT '{}'::jsonb,
			%s VECTOR(%d)
		)`,
		table, embeddingColumn, dims,
	)
	_, err := t.db.ExecContext(ctx, query)
	return err
}

// VectorIndexOptions configures vector index creation for CockroachDB v26.2+.
type VectorIndexOptions struct {
	// Metric sets the distance function for the index. Default is Cosine.
	Metric DistanceMetric
	// PrefixColumns are columns that partition the index for filtered searches.
	// They must be existing columns on the table and appear before the vector column
	// in the CREATE VECTOR INDEX statement.
	PrefixColumns []string
}

// InitSchemaV26 creates the vector table, enables the vector_index feature flag,
// and creates a vector index using the CockroachDB v26.2+ CREATE VECTOR INDEX syntax.
//
// The vector index uses the new syntax:
//
//	CREATE VECTOR INDEX ON table (embedding vector_cosine_ops)
//
// With prefix columns for partitioned searches:
//
//	CREATE VECTOR INDEX ON table (category, embedding vector_cosine_ops)
//
// Set idxOpts to nil to skip vector index creation (table-only).
func (t *VectorSearchTemplate) InitSchemaV26(ctx context.Context, table, embeddingColumn string, dims int, idxOpts *VectorIndexOptions) error {
	if err := validateIdentifier(table); err != nil {
		return fmt.Errorf("ai: invalid table name: %w", err)
	}
	if err := validateIdentifier(embeddingColumn); err != nil {
		return fmt.Errorf("ai: invalid column name: %w", err)
	}
	if dims <= 0 {
		return errors.New("ai: dimensions must be positive")
	}

	// Create the table.
	createTable := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			content TEXT,
			metadata JSONB DEFAULT '{}'::jsonb,
			%s VECTOR(%d)
		)`,
		table, embeddingColumn, dims,
	)
	if _, err := t.db.ExecContext(ctx, createTable); err != nil {
		return fmt.Errorf("ai: create table: %w", err)
	}

	// Create vector index if requested.
	if idxOpts != nil {
		// Enable the vector_index feature flag (required for v26.2).
		if _, err := t.db.ExecContext(ctx, "SET CLUSTER SETTING feature.vector_index.enabled = true"); err != nil {
			return fmt.Errorf("ai: enable vector_index feature: %w", err)
		}

		metric := idxOpts.Metric
		opClass := metric.OpClass()

		// Validate prefix columns.
		for _, col := range idxOpts.PrefixColumns {
			if err := validateIdentifier(col); err != nil {
				return fmt.Errorf("ai: invalid prefix column %q: %w", col, err)
			}
		}

		// Build column list: prefix columns + embedding column with opclass.
		var colParts []string
		for _, col := range idxOpts.PrefixColumns {
			colParts = append(colParts, col)
		}
		colParts = append(colParts, fmt.Sprintf("%s %s", embeddingColumn, opClass))

		createIndex := fmt.Sprintf(
			"CREATE VECTOR INDEX IF NOT EXISTS %s_vec_idx ON %s (%s)",
			table, table, strings.Join(colParts, ", "),
		)
		if _, err := t.db.ExecContext(ctx, createIndex); err != nil {
			return fmt.Errorf("ai: create vector index: %w", err)
		}
	}

	return nil
}

// Search performs a vector similarity search within a read-only transaction.
// The beamSize parameter tunes C-SPANN accuracy (0 = CockroachDB default).
func (t *VectorSearchTemplate) Search(ctx context.Context, opts SearchOptions) ([]VectorSearchResult, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}

	startNanos := time.Now().UnixNano()
	vectorLit := ToVectorLiteral(opts.QueryVector)
	op := opts.Metric.Operator()

	tx, err := t.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.metrics.RecordVectorSearch(startNanos, 0, false)
		return nil, fmt.Errorf("ai: begin tx: %w", err)
	}
	defer tx.Rollback()

	if opts.BeamSize > 0 {
		_, err = tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL vector_search_beam_size = %d", opts.BeamSize))
		if err != nil {
			t.metrics.RecordVectorSearch(startNanos, 0, false)
			return nil, fmt.Errorf("ai: set beam_size: %w", err)
		}
	}

	cols := "*"
	if len(opts.Columns) > 0 {
		cols = strings.Join(opts.Columns, ", ")
	}

	query := fmt.Sprintf(
		"SELECT %s, %s %s '%s'::vector AS distance FROM %s",
		cols, opts.VectorColumn, op, vectorLit, opts.Table,
	)

	if opts.Filter != "" {
		query += " WHERE " + opts.Filter
	}

	query += fmt.Sprintf(
		" ORDER BY %s %s '%s'::vector LIMIT %d",
		opts.VectorColumn, op, vectorLit, opts.Limit,
	)

	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		t.metrics.RecordVectorSearch(startNanos, 0, false)
		return nil, fmt.Errorf("ai: search query: %w", err)
	}
	defer rows.Close()

	results, err := t.scanSearchResults(rows, opts.Columns)
	if err != nil {
		t.metrics.RecordVectorSearch(startNanos, 0, false)
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		t.metrics.RecordVectorSearch(startNanos, 0, false)
		return nil, fmt.Errorf("ai: commit: %w", err)
	}

	t.metrics.RecordVectorSearch(startNanos, len(results), true)
	return results, nil
}

func (t *VectorSearchTemplate) scanSearchResults(rows *sql.Rows, _ []string) ([]VectorSearchResult, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("ai: columns: %w", err)
	}

	var results []VectorSearchResult
	for rows.Next() {
		values := make([]any, len(cols))
		valuePtrs := make([]any, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("ai: scan: %w", err)
		}

		result := VectorSearchResult{
			Metadata: make(map[string]any),
		}

		for i, colName := range cols {
			val := values[i]
			switch colName {
			case "distance":
				result.Distance = toFloat64(val)
			case "id":
				result.ID = toString(val)
			default:
				result.Metadata[colName] = val
			}
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ai: rows iteration: %w", err)
	}
	return results, nil
}

// StoreEmbedding upserts a vector embedding with optional metadata.
// The embedding is stored using CockroachDB's native VECTOR type with INSERT...ON CONFLICT.
func (t *VectorSearchTemplate) StoreEmbedding(ctx context.Context, opts StoreOptions) error {
	if err := opts.validate(); err != nil {
		return err
	}

	vectorLit := ToVectorLiteral(opts.Embedding)

	if len(opts.Metadata) == 0 {
		query := fmt.Sprintf(
			"INSERT INTO %s (%s, %s) VALUES ($1, '%s'::vector) "+
				"ON CONFLICT (%s) DO UPDATE SET %s = '%s'::vector",
			opts.Table, opts.IDColumn, opts.VectorColumn, vectorLit,
			opts.IDColumn, opts.VectorColumn, vectorLit,
		)
		_, err := t.db.ExecContext(ctx, query, opts.ID)
		return err
	}

	metaCols := make([]string, 0, len(opts.Metadata))
	metaVals := make([]any, 0, len(opts.Metadata))
	metaUpdates := make([]string, 0, len(opts.Metadata))
	argIdx := 2

	for col, val := range opts.Metadata {
		if err := validateIdentifier(col); err != nil {
			return fmt.Errorf("ai: invalid metadata column %q: %w", col, err)
		}
		metaCols = append(metaCols, col)
		metaVals = append(metaVals, val)
		metaUpdates = append(metaUpdates, fmt.Sprintf("%s = $%d", col, argIdx))
		argIdx++
	}

	allCols := fmt.Sprintf("%s, %s, %s", opts.IDColumn, opts.VectorColumn, strings.Join(metaCols, ", "))
	var phBuilder strings.Builder
	phBuilder.WriteString("$1, '")
	phBuilder.WriteString(vectorLit)
	phBuilder.WriteString("'::vector")
	for i := range metaCols {
		fmt.Fprintf(&phBuilder, ", $%d", i+2)
	}
	placeholders := phBuilder.String()
	updates := fmt.Sprintf("%s = '%s'::vector, %s", opts.VectorColumn, vectorLit, strings.Join(metaUpdates, ", "))

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s",
		opts.Table, allCols, placeholders, opts.IDColumn, updates,
	)

	args := make([]any, 0, 1+len(metaVals))
	args = append(args, opts.ID)
	args = append(args, metaVals...)

	_, err := t.db.ExecContext(ctx, query, args...)
	return err
}

// SearchOptions configures a vector search operation.
type SearchOptions struct {
	Table        string
	VectorColumn string
	QueryVector  []float32
	Metric       DistanceMetric
	Limit        int
	Columns      []string
	Filter       string
	BeamSize     int
}

func (o SearchOptions) validate() error {
	if o.Table == "" {
		return errors.New("ai: table is required")
	}
	if o.VectorColumn == "" {
		return errors.New("ai: vector column is required")
	}
	if len(o.QueryVector) == 0 {
		return errors.New("ai: query vector is required")
	}
	if o.Limit <= 0 {
		return errors.New("ai: limit must be positive")
	}
	return nil
}

// StoreOptions configures a vector store operation.
type StoreOptions struct {
	Table        string
	IDColumn     string
	VectorColumn string
	ID           string
	Embedding    []float32
	Metadata     map[string]any
}

func (o StoreOptions) validate() error {
	if o.Table == "" {
		return errors.New("ai: table is required")
	}
	if o.IDColumn == "" {
		return errors.New("ai: id column is required")
	}
	if o.VectorColumn == "" {
		return errors.New("ai: vector column is required")
	}
	if o.ID == "" {
		return errors.New("ai: id is required")
	}
	if len(o.Embedding) == 0 {
		return errors.New("ai: embedding is required")
	}
	return nil
}

func validateIdentifier(name string) error {
	if name == "" {
		return errors.New("identifier is empty")
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return fmt.Errorf("identifier %q contains invalid character %q", name, c)
		}
	}
	return nil
}

func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int64:
		return float64(val)
	case []byte:
		var f float64
		fmt.Sscanf(string(val), "%f", &f)
		return f
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	default:
		return 0
	}
}

func toString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// metadataToJSON serializes metadata to a JSON string for JSONB storage.
func metadataToJSON(m map[string]any) (string, error) {
	if m == nil {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("ai: marshal metadata: %w", err)
	}
	return string(b), nil
}
