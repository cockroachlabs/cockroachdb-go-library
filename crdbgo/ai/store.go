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

// VectorStore provides high-level document storage with automatic embedding
// generation. For high-level operations (add, delete, similarity search), use
// this type. For low-level SQL control, use [VectorSearchTemplate] instead.
type VectorStore struct {
	db               SQLDB
	embedder         EmbeddingGenerator
	metrics          AIMetrics
	tableName        string
	contentColumn    string
	embeddingColumn  string
	metadataColumn   string
	distanceMetric   DistanceMetric
	beamSize         int
	dimensions       int
	initializeSchema bool
	initialized      bool
	createVectorIndex bool
	prefixColumns    []string
}

// VectorStoreOption configures a [VectorStore]. Pass options to [NewVectorStore].
type VectorStoreOption func(*VectorStore)

// NewVectorStore creates a high-level document store backed by CockroachDB's
// native VECTOR type. The embedder is used to auto-embed documents that lack
// a pre-computed embedding. Defaults to table "vector_store" with cosine distance.
func NewVectorStore(db SQLDB, embedder EmbeddingGenerator, opts ...VectorStoreOption) *VectorStore {
	s := &VectorStore{
		db:              db,
		embedder:        embedder,
		metrics:         &noopAIMetrics{},
		tableName:       "vector_store",
		contentColumn:   "content",
		embeddingColumn: "embedding",
		metadataColumn:  "metadata",
		distanceMetric:  Cosine,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// WithStoreMetrics attaches an [AIMetrics] recorder to the store.
func WithStoreMetrics(m AIMetrics) VectorStoreOption {
	return func(s *VectorStore) {
		if m != nil {
			s.metrics = m
		}
	}
}

// WithTableName sets the table name for vector storage. Default is "vector_store".
func WithTableName(name string) VectorStoreOption {
	return func(s *VectorStore) { s.tableName = name }
}

// WithContentColumn sets the text content column name. Default is "content".
func WithContentColumn(name string) VectorStoreOption {
	return func(s *VectorStore) { s.contentColumn = name }
}

// WithEmbeddingColumn sets the VECTOR column name. Default is "embedding".
func WithEmbeddingColumn(name string) VectorStoreOption {
	return func(s *VectorStore) { s.embeddingColumn = name }
}

// WithMetadataColumn sets the JSONB metadata column name. Default is "metadata".
func WithMetadataColumn(name string) VectorStoreOption {
	return func(s *VectorStore) { s.metadataColumn = name }
}

// WithDistanceMetric sets the vector distance function ([L2], [Cosine], or
// [InnerProduct]). Default is [Cosine].
func WithDistanceMetric(m DistanceMetric) VectorStoreOption {
	return func(s *VectorStore) { s.distanceMetric = m }
}

// WithBeamSize sets the C-SPANN beam size for search accuracy tuning.
// Higher values = more accurate but slower. 0 = CockroachDB default.
func WithBeamSize(n int) VectorStoreOption {
	return func(s *VectorStore) { s.beamSize = n }
}

// WithDimensions sets the vector dimensionality (e.g., 1536 for OpenAI ada-002).
// If not set and [WithInitializeSchema] is true, dimensions are inferred from
// the [EmbeddingGenerator].
func WithDimensions(d int) VectorStoreOption {
	return func(s *VectorStore) { s.dimensions = d }
}

// WithInitializeSchema enables automatic CREATE TABLE IF NOT EXISTS on first
// use. Default is false (table must already exist).
func WithInitializeSchema(init bool) VectorStoreOption {
	return func(s *VectorStore) { s.initializeSchema = init }
}

// WithCreateVectorIndex enables automatic CREATE VECTOR INDEX creation using
// the CockroachDB v26.2+ syntax when [WithInitializeSchema] is also true.
// This also enables the feature.vector_index.enabled cluster setting.
func WithCreateVectorIndex(create bool) VectorStoreOption {
	return func(s *VectorStore) { s.createVectorIndex = create }
}

// WithPrefixColumns sets the prefix columns for partitioned vector index searches
// in CockroachDB v26.2+. These columns appear before the embedding column in the
// CREATE VECTOR INDEX statement and must be existing columns on the table.
func WithPrefixColumns(cols ...string) VectorStoreOption {
	return func(s *VectorStore) { s.prefixColumns = cols }
}

func (s *VectorStore) ensureSchema(ctx context.Context) error {
	if !s.initializeSchema || s.initialized {
		return nil
	}
	dims := s.dimensions
	if dims == 0 && s.embedder != nil {
		dims = s.embedder.Dimensions()
	}
	if dims <= 0 {
		return errors.New("ai: dimensions must be positive for schema initialization")
	}

	query := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			%s TEXT,
			%s JSONB DEFAULT '{}'::jsonb,
			%s VECTOR(%d)
		)`,
		s.tableName, s.contentColumn, s.metadataColumn, s.embeddingColumn, dims,
	)
	if _, err := s.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("ai: init schema: %w", err)
	}

	// CockroachDB v26.2+: create vector index if requested.
	if s.createVectorIndex {
		if _, err := s.db.ExecContext(ctx, "SET CLUSTER SETTING feature.vector_index.enabled = true"); err != nil {
			return fmt.Errorf("ai: enable vector_index feature: %w", err)
		}

		opClass := s.distanceMetric.OpClass()

		var colParts []string
		for _, col := range s.prefixColumns {
			colParts = append(colParts, col)
		}
		colParts = append(colParts, fmt.Sprintf("%s %s", s.embeddingColumn, opClass))

		createIndex := fmt.Sprintf(
			"CREATE VECTOR INDEX IF NOT EXISTS %s_vec_idx ON %s (%s)",
			s.tableName, s.tableName, strings.Join(colParts, ", "),
		)
		if _, err := s.db.ExecContext(ctx, createIndex); err != nil {
			return fmt.Errorf("ai: create vector index: %w", err)
		}
	}

	s.initialized = true
	return nil
}

// AddDocuments embeds and stores documents. If a document's Embedding is nil,
// it is auto-generated from Text via the EmbeddingGenerator.
func (s *VectorStore) AddDocuments(ctx context.Context, docs []VectorDocument) error {
	if err := s.ensureSchema(ctx); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ai: begin tx: %w", err)
	}
	defer tx.Rollback()

	for i := range docs {
		doc := &docs[i]
		if doc.ID == "" {
			doc.ID = newUUID()
		}
		if len(doc.Embedding) == 0 {
			if s.embedder == nil {
				return errors.New("ai: no embedding generator and document has no embedding")
			}
			startNanos := time.Now().UnixNano()
			emb, err := s.embedder.GenerateEmbedding(ctx, doc.Text)
			if err != nil {
				s.metrics.RecordEmbeddingGeneration(startNanos, 0, false)
				return fmt.Errorf("ai: embed document %q: %w", doc.ID, err)
			}
			doc.Embedding = emb
			s.metrics.RecordEmbeddingGeneration(startNanos, len(doc.Text), true)
		}

		metaJSON, err := metadataToJSON(doc.Metadata)
		if err != nil {
			return err
		}

		vectorLit := ToVectorLiteral(doc.Embedding)
		query := fmt.Sprintf(
			`INSERT INTO %s (id, %s, %s, %s) VALUES ($1, $2, $3::jsonb, '%s'::vector)
			ON CONFLICT (id) DO UPDATE SET
				%s = EXCLUDED.%s,
				%s = EXCLUDED.%s,
				%s = EXCLUDED.%s`,
			s.tableName, s.contentColumn, s.metadataColumn, s.embeddingColumn, vectorLit,
			s.contentColumn, s.contentColumn,
			s.metadataColumn, s.metadataColumn,
			s.embeddingColumn, s.embeddingColumn,
		)
		if _, err := tx.ExecContext(ctx, query, doc.ID, doc.Text, metaJSON); err != nil {
			return fmt.Errorf("ai: insert document %q: %w", doc.ID, err)
		}
	}

	return tx.Commit()
}

// DeleteDocuments removes documents by their IDs.
func (s *VectorStore) DeleteDocuments(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	query := fmt.Sprintf("DELETE FROM %s WHERE id IN (%s)", s.tableName, strings.Join(placeholders, ", "))
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

// SimilaritySearch embeds the query text via the [EmbeddingGenerator] and
// searches for similar documents using CockroachDB's native vector operators.
// Results are ordered by similarity (highest first) and filtered by threshold.
func (s *VectorStore) SimilaritySearch(ctx context.Context, query string, opts ...SimilarityOption) ([]VectorDocument, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return nil, err
	}

	cfg := similarityConfig{
		topK:                10,
		similarityThreshold: 0.0,
	}
	for _, o := range opts {
		o(&cfg)
	}

	if s.embedder == nil {
		return nil, errors.New("ai: no embedding generator for similarity search")
	}

	startNanos := time.Now().UnixNano()
	queryVector, err := s.embedder.GenerateEmbedding(ctx, query)
	if err != nil {
		s.metrics.RecordEmbeddingGeneration(startNanos, 0, false)
		return nil, fmt.Errorf("ai: embed query: %w", err)
	}
	s.metrics.RecordEmbeddingGeneration(startNanos, len(query), true)

	searchStart := time.Now().UnixNano()
	vectorLit := ToVectorLiteral(queryVector)
	op := s.distanceMetric.Operator()

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		s.metrics.RecordVectorSearch(searchStart, 0, false)
		return nil, fmt.Errorf("ai: begin tx: %w", err)
	}
	defer tx.Rollback()

	if s.beamSize > 0 {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL vector_search_beam_size = %d", s.beamSize)); err != nil {
			s.metrics.RecordVectorSearch(searchStart, 0, false)
			return nil, fmt.Errorf("ai: set beam_size: %w", err)
		}
	}

	sqlQuery := fmt.Sprintf(
		"SELECT id, %s, %s, %s %s '%s'::vector AS distance FROM %s",
		s.contentColumn, s.metadataColumn, s.embeddingColumn, op, vectorLit, s.tableName,
	)

	if cfg.filterExpression != "" {
		sqlQuery += " WHERE " + cfg.filterExpression
	}

	sqlQuery += fmt.Sprintf(
		" ORDER BY %s %s '%s'::vector LIMIT %d",
		s.embeddingColumn, op, vectorLit, cfg.topK,
	)

	rows, err := tx.QueryContext(ctx, sqlQuery)
	if err != nil {
		s.metrics.RecordVectorSearch(searchStart, 0, false)
		return nil, fmt.Errorf("ai: similarity search: %w", err)
	}
	defer rows.Close()

	var docs []VectorDocument
	for rows.Next() {
		var doc VectorDocument
		var metaJSON string
		var distance float64
		if err := rows.Scan(&doc.ID, &doc.Text, &metaJSON, &distance); err != nil {
			s.metrics.RecordVectorSearch(searchStart, 0, false)
			return nil, fmt.Errorf("ai: scan: %w", err)
		}
		if metaJSON != "" {
			if err := json.Unmarshal([]byte(metaJSON), &doc.Metadata); err != nil {
				doc.Metadata = map[string]any{"raw": metaJSON}
			}
		}
		doc.Score = s.distanceMetric.ToSimilarity(distance)

		if cfg.similarityThreshold > 0 && doc.Score < cfg.similarityThreshold {
			continue
		}
		docs = append(docs, doc)
	}
	if err := rows.Err(); err != nil {
		s.metrics.RecordVectorSearch(searchStart, 0, false)
		return nil, fmt.Errorf("ai: rows: %w", err)
	}

	if err := tx.Commit(); err != nil {
		s.metrics.RecordVectorSearch(searchStart, 0, false)
		return nil, fmt.Errorf("ai: commit: %w", err)
	}

	s.metrics.RecordVectorSearch(searchStart, len(docs), true)
	return docs, nil
}

// SimilarityOption configures a [VectorStore.SimilaritySearch] call.
type SimilarityOption func(*similarityConfig)

type similarityConfig struct {
	topK                int
	similarityThreshold float64
	filterExpression    string
}

// WithTopK sets the maximum number of results to return.
func WithTopK(k int) SimilarityOption {
	return func(c *similarityConfig) { c.topK = k }
}

// WithSimilarityThreshold filters results below this similarity score.
func WithSimilarityThreshold(t float64) SimilarityOption {
	return func(c *similarityConfig) { c.similarityThreshold = t }
}

// WithFilterExpression adds a SQL WHERE clause to narrow results.
func WithFilterExpression(expr string) SimilarityOption {
	return func(c *similarityConfig) { c.filterExpression = expr }
}
