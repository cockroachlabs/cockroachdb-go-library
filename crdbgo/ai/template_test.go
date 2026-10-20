package ai

import (
	"context"
	"strings"
	"testing"
)

func TestNewVectorSearchTemplate(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)
	if tmpl == nil {
		t.Fatal("expected non-nil template")
	}
	if tmpl.db != db {
		t.Error("db not set")
	}
}

func TestNewVectorSearchTemplate_WithMetrics(t *testing.T) {
	db := newMockDB()
	m := NewInMemoryAIMetrics()
	tmpl := NewVectorSearchTemplate(db, WithTemplateMetrics(m))
	if tmpl.metrics != m {
		t.Error("metrics not set")
	}
}

func TestNewVectorSearchTemplate_NilMetrics(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db, WithTemplateMetrics(nil))
	if _, ok := tmpl.metrics.(*noopAIMetrics); !ok {
		t.Error("nil metrics should keep noop default")
	}
}

func TestInitSchema_ValidInput(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)

	err := tmpl.InitSchema(context.Background(), "documents", "embedding", 1536)
	if err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	queries := db.queries()
	if len(queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(queries))
	}
	if !strings.Contains(queries[0], "CREATE TABLE IF NOT EXISTS documents") {
		t.Errorf("missing CREATE TABLE, got: %s", queries[0])
	}
	if !strings.Contains(queries[0], "VECTOR(1536)") {
		t.Errorf("missing VECTOR(1536), got: %s", queries[0])
	}
	if !strings.Contains(queries[0], "JSONB") {
		t.Errorf("missing JSONB column, got: %s", queries[0])
	}
}

func TestInitSchema_InvalidTable(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)
	err := tmpl.InitSchema(context.Background(), "drop;table", "embedding", 1536)
	if err == nil {
		t.Error("expected error for invalid table name")
	}
}

func TestInitSchema_EmptyTable(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)
	err := tmpl.InitSchema(context.Background(), "", "embedding", 1536)
	if err == nil {
		t.Error("expected error for empty table name")
	}
}

func TestInitSchema_ZeroDimensions(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)
	err := tmpl.InitSchema(context.Background(), "docs", "embedding", 0)
	if err == nil {
		t.Error("expected error for zero dimensions")
	}
}

// --- v26.2 InitSchemaV26 tests ---

func TestInitSchemaV26_TableOnly(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)

	err := tmpl.InitSchemaV26(context.Background(), "documents", "embedding", 1536, nil)
	if err != nil {
		t.Fatalf("InitSchemaV26 (table only) failed: %v", err)
	}

	queries := db.queries()
	if len(queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(queries))
	}
	if !strings.Contains(queries[0], "CREATE TABLE IF NOT EXISTS documents") {
		t.Errorf("missing CREATE TABLE, got: %s", queries[0])
	}
	if !strings.Contains(queries[0], "VECTOR(1536)") {
		t.Errorf("missing VECTOR(1536), got: %s", queries[0])
	}
}

func TestInitSchemaV26_WithVectorIndex_Cosine(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)

	err := tmpl.InitSchemaV26(context.Background(), "documents", "embedding", 1536, &VectorIndexOptions{
		Metric: Cosine,
	})
	if err != nil {
		t.Fatalf("InitSchemaV26 failed: %v", err)
	}

	queries := db.queries()
	if len(queries) != 3 {
		t.Fatalf("expected 3 queries (CREATE TABLE, SET CLUSTER SETTING, CREATE VECTOR INDEX), got %d", len(queries))
	}
	if !strings.Contains(queries[1], "feature.vector_index.enabled = true") {
		t.Errorf("missing feature flag, got: %s", queries[1])
	}
	if !strings.Contains(queries[2], "CREATE VECTOR INDEX") {
		t.Errorf("missing CREATE VECTOR INDEX, got: %s", queries[2])
	}
	if !strings.Contains(queries[2], "vector_cosine_ops") {
		t.Errorf("missing vector_cosine_ops opclass, got: %s", queries[2])
	}
	if !strings.Contains(queries[2], "documents_vec_idx ON documents") {
		t.Errorf("wrong index name or table, got: %s", queries[2])
	}
}

func TestInitSchemaV26_WithVectorIndex_L2(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)

	err := tmpl.InitSchemaV26(context.Background(), "docs", "emb", 768, &VectorIndexOptions{
		Metric: L2,
	})
	if err != nil {
		t.Fatalf("InitSchemaV26 failed: %v", err)
	}

	queries := db.queries()
	if !strings.Contains(queries[2], "vector_l2_ops") {
		t.Errorf("missing vector_l2_ops opclass, got: %s", queries[2])
	}
}

func TestInitSchemaV26_WithVectorIndex_InnerProduct(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)

	err := tmpl.InitSchemaV26(context.Background(), "docs", "emb", 768, &VectorIndexOptions{
		Metric: InnerProduct,
	})
	if err != nil {
		t.Fatalf("InitSchemaV26 failed: %v", err)
	}

	queries := db.queries()
	if !strings.Contains(queries[2], "vector_ip_ops") {
		t.Errorf("missing vector_ip_ops opclass, got: %s", queries[2])
	}
}

func TestInitSchemaV26_WithPrefixColumns(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)

	err := tmpl.InitSchemaV26(context.Background(), "documents", "embedding", 1536, &VectorIndexOptions{
		Metric:        Cosine,
		PrefixColumns: []string{"category", "tenant_id"},
	})
	if err != nil {
		t.Fatalf("InitSchemaV26 failed: %v", err)
	}

	queries := db.queries()
	if len(queries) != 3 {
		t.Fatalf("expected 3 queries, got %d", len(queries))
	}
	idx := queries[2]
	if !strings.Contains(idx, "category, tenant_id, embedding vector_cosine_ops") {
		t.Errorf("missing prefix columns in index DDL, got: %s", idx)
	}
}

func TestInitSchemaV26_InvalidPrefixColumn(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)

	err := tmpl.InitSchemaV26(context.Background(), "docs", "embedding", 768, &VectorIndexOptions{
		Metric:        Cosine,
		PrefixColumns: []string{"bad;col"},
	})
	if err == nil {
		t.Error("expected error for invalid prefix column name")
	}
}

func TestInitSchemaV26_ZeroDimensions(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)
	err := tmpl.InitSchemaV26(context.Background(), "docs", "embedding", 0, nil)
	if err == nil {
		t.Error("expected error for zero dimensions")
	}
}

func TestInitSchemaV26_InvalidTable(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)
	err := tmpl.InitSchemaV26(context.Background(), "drop;table", "embedding", 768, nil)
	if err == nil {
		t.Error("expected error for invalid table name")
	}
}

func TestSearch_Validation(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)

	tests := []struct {
		name string
		opts SearchOptions
	}{
		{"empty table", SearchOptions{VectorColumn: "emb", QueryVector: []float32{1}, Limit: 10}},
		{"empty column", SearchOptions{Table: "docs", QueryVector: []float32{1}, Limit: 10}},
		{"empty vector", SearchOptions{Table: "docs", VectorColumn: "emb", Limit: 10}},
		{"zero limit", SearchOptions{Table: "docs", VectorColumn: "emb", QueryVector: []float32{1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tmpl.Search(context.Background(), tt.opts)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestSearch_WithFakeDB(t *testing.T) {
	db := getFakeDB(t)
	tmpl := NewVectorSearchTemplate(db)

	results, err := tmpl.Search(context.Background(), SearchOptions{
		Table:        "documents",
		VectorColumn: "embedding",
		QueryVector:  []float32{0.1, 0.2, 0.3},
		Metric:       Cosine,
		Limit:        5,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "doc-1" {
		t.Errorf("ID = %q, want %q", results[0].ID, "doc-1")
	}
	if results[0].Distance != 0.15 {
		t.Errorf("Distance = %f, want 0.15", results[0].Distance)
	}
}

func TestSearch_WithBeamSize(t *testing.T) {
	db := getFakeDB(t)
	tmpl := NewVectorSearchTemplate(db)

	results, err := tmpl.Search(context.Background(), SearchOptions{
		Table:        "documents",
		VectorColumn: "embedding",
		QueryVector:  []float32{0.1, 0.2},
		Metric:       L2,
		Limit:        10,
		BeamSize:     32,
	})
	if err != nil {
		t.Fatalf("Search with beam size failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected results")
	}
}

func TestSearch_WithColumns(t *testing.T) {
	db := getFakeDB(t)
	tmpl := NewVectorSearchTemplate(db)

	_, err := tmpl.Search(context.Background(), SearchOptions{
		Table:        "documents",
		VectorColumn: "embedding",
		QueryVector:  []float32{0.1, 0.2},
		Metric:       InnerProduct,
		Limit:        5,
		Columns:      []string{"id", "title"},
	})
	if err != nil {
		t.Fatalf("Search with columns failed: %v", err)
	}
}

func TestSearch_WithFilter(t *testing.T) {
	db := getFakeDB(t)
	tmpl := NewVectorSearchTemplate(db)

	_, err := tmpl.Search(context.Background(), SearchOptions{
		Table:        "documents",
		VectorColumn: "embedding",
		QueryVector:  []float32{0.1},
		Metric:       Cosine,
		Limit:        5,
		Filter:       "category = 'science'",
	})
	if err != nil {
		t.Fatalf("Search with filter failed: %v", err)
	}
}

func TestSearch_MetricsRecorded(t *testing.T) {
	db := getFakeDB(t)
	m := NewInMemoryAIMetrics()
	tmpl := NewVectorSearchTemplate(db, WithTemplateMetrics(m))

	_, err := tmpl.Search(context.Background(), SearchOptions{
		Table:        "documents",
		VectorColumn: "embedding",
		QueryVector:  []float32{0.1, 0.2},
		Metric:       Cosine,
		Limit:        5,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if m.VectorSearchCount() != 1 {
		t.Errorf("search count = %d, want 1", m.VectorSearchCount())
	}
	if m.VectorSearchFailureCount() != 0 {
		t.Errorf("failure count = %d, want 0", m.VectorSearchFailureCount())
	}
}

func TestStoreEmbedding_Validation(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)

	tests := []struct {
		name string
		opts StoreOptions
	}{
		{"empty table", StoreOptions{IDColumn: "id", VectorColumn: "emb", ID: "1", Embedding: []float32{1}}},
		{"empty id_col", StoreOptions{Table: "docs", VectorColumn: "emb", ID: "1", Embedding: []float32{1}}},
		{"empty vec_col", StoreOptions{Table: "docs", IDColumn: "id", ID: "1", Embedding: []float32{1}}},
		{"empty id", StoreOptions{Table: "docs", IDColumn: "id", VectorColumn: "emb", Embedding: []float32{1}}},
		{"empty embedding", StoreOptions{Table: "docs", IDColumn: "id", VectorColumn: "emb", ID: "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tmpl.StoreEmbedding(context.Background(), tt.opts)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestStoreEmbedding_Simple(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)

	err := tmpl.StoreEmbedding(context.Background(), StoreOptions{
		Table:        "documents",
		IDColumn:     "id",
		VectorColumn: "embedding",
		ID:           "doc-1",
		Embedding:    []float32{0.1, 0.2, 0.3},
	})
	if err != nil {
		t.Fatalf("StoreEmbedding failed: %v", err)
	}

	queries := db.queries()
	if len(queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(queries))
	}
	if !strings.Contains(queries[0], "INSERT INTO documents") {
		t.Errorf("missing INSERT, got: %s", queries[0])
	}
	if !strings.Contains(queries[0], "ON CONFLICT") {
		t.Errorf("missing ON CONFLICT, got: %s", queries[0])
	}
	if !strings.Contains(queries[0], "[0.1,0.2,0.3]") {
		t.Errorf("missing vector literal, got: %s", queries[0])
	}
}

func TestStoreEmbedding_WithMetadata(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)

	err := tmpl.StoreEmbedding(context.Background(), StoreOptions{
		Table:        "documents",
		IDColumn:     "id",
		VectorColumn: "embedding",
		ID:           "doc-1",
		Embedding:    []float32{0.1},
		Metadata:     map[string]any{"title": "Test Doc"},
	})
	if err != nil {
		t.Fatalf("StoreEmbedding with metadata failed: %v", err)
	}

	queries := db.queries()
	if len(queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(queries))
	}
	if !strings.Contains(queries[0], "title") {
		t.Errorf("missing metadata column, got: %s", queries[0])
	}
}

func TestStoreEmbedding_InvalidMetadataColumn(t *testing.T) {
	db := newMockDB()
	tmpl := NewVectorSearchTemplate(db)

	err := tmpl.StoreEmbedding(context.Background(), StoreOptions{
		Table:        "documents",
		IDColumn:     "id",
		VectorColumn: "embedding",
		ID:           "doc-1",
		Embedding:    []float32{0.1},
		Metadata:     map[string]any{"bad;col": "value"},
	})
	if err == nil {
		t.Error("expected error for SQL-injection metadata column name")
	}
}

func TestValidateIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "my_table", false},
		{"valid_caps", "MyTable123", false},
		{"empty", "", true},
		{"semicolon", "drop;table", true},
		{"space", "my table", true},
		{"dash", "my-table", true},
		{"paren", "func()", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIdentifier(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateIdentifier(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
