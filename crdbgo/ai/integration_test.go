//go:build integration

package ai_test

import (
	"context"
	"math"
	"testing"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/ai"
	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/testutil"
)

// mockEmbedder returns fixed-dimension embeddings based on simple text hashing.
// It produces deterministic vectors for testing.
type mockEmbedder struct {
	dims int
}

func (m *mockEmbedder) GenerateEmbedding(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, m.dims)
	for i, c := range text {
		vec[i%m.dims] += float32(c) / 1000.0
	}
	// Normalize to unit length for cosine similarity to be meaningful.
	var norm float32
	for _, v := range vec {
		norm += v * v
	}
	if norm > 0 {
		norm = float32(math.Sqrt(float64(norm)))
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec, nil
}

func (m *mockEmbedder) Dimensions() int { return m.dims }

// --- VectorSearchTemplate tests ---

func TestIntegration_Template_InitSchema(t *testing.T) {
	db := testutil.SqlDB(t)
	ctx := context.Background()
	table := testutil.UniqueTable("ai_schema")

	tmpl := ai.NewVectorSearchTemplate(db)

	if err := tmpl.InitSchema(ctx, table, "embedding", 3); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Verify table exists by querying it.
	rows, err := db.QueryContext(ctx, "SELECT column_name FROM information_schema.columns WHERE table_name = $1 ORDER BY ordinal_position", table)
	if err != nil {
		t.Fatalf("query columns failed: %v", err)
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		cols = append(cols, col)
	}

	// Expect: id, content, metadata, embedding
	if len(cols) < 3 {
		t.Errorf("expected at least 3 columns (id, content, metadata, embedding), got %v", cols)
	}
}

func TestIntegration_Template_StoreAndSearch(t *testing.T) {
	db := testutil.SqlDB(t)
	ctx := context.Background()
	table := testutil.UniqueTable("ai_store_search")

	tmpl := ai.NewVectorSearchTemplate(db)

	// Init schema with 3 dimensions.
	if err := tmpl.InitSchema(ctx, table, "embedding", 3); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Store embeddings — three orthogonal unit vectors.
	vectors := []struct {
		id  string
		emb []float32
	}{
		{"vec-x", []float32{1, 0, 0}},
		{"vec-y", []float32{0, 1, 0}},
		{"vec-z", []float32{0, 0, 1}},
	}

	for _, v := range vectors {
		err := tmpl.StoreEmbedding(ctx, ai.StoreOptions{
			Table:        table,
			IDColumn:     "id",
			VectorColumn: "embedding",
			ID:           v.id,
			Embedding:    v.emb,
		})
		if err != nil {
			t.Fatalf("StoreEmbedding(%s) failed: %v", v.id, err)
		}
	}

	// Search with L2 distance — query [1,0,0] should return vec-x as closest.
	results, err := tmpl.Search(ctx, ai.SearchOptions{
		Table:        table,
		VectorColumn: "embedding",
		QueryVector:  []float32{1, 0, 0},
		Metric:       ai.L2,
		Limit:        3,
		Columns:      []string{"id"},
	})
	if err != nil {
		t.Fatalf("Search L2 failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search L2 returned no results")
	}
	if results[0].ID != "vec-x" {
		t.Errorf("L2: expected closest to be vec-x, got %q", results[0].ID)
	}
}

func TestIntegration_Template_Search_Cosine(t *testing.T) {
	db := testutil.SqlDB(t)
	ctx := context.Background()
	table := testutil.UniqueTable("ai_cosine")

	tmpl := ai.NewVectorSearchTemplate(db)
	if err := tmpl.InitSchema(ctx, table, "embedding", 3); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Store vectors.
	for _, v := range []struct {
		id  string
		emb []float32
	}{
		{"a", []float32{1, 0, 0}},
		{"b", []float32{0.9, 0.1, 0}},
		{"c", []float32{0, 0, 1}},
	} {
		if err := tmpl.StoreEmbedding(ctx, ai.StoreOptions{
			Table: table, IDColumn: "id", VectorColumn: "embedding",
			ID: v.id, Embedding: v.emb,
		}); err != nil {
			t.Fatalf("StoreEmbedding failed: %v", err)
		}
	}

	results, err := tmpl.Search(ctx, ai.SearchOptions{
		Table:        table,
		VectorColumn: "embedding",
		QueryVector:  []float32{1, 0, 0},
		Metric:       ai.Cosine,
		Limit:        3,
		Columns:      []string{"id"},
	})
	if err != nil {
		t.Fatalf("Search Cosine failed: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	// The closest by cosine to [1,0,0] should be "a" (distance 0), then "b".
	if results[0].ID != "a" {
		t.Errorf("Cosine: expected closest to be 'a', got %q", results[0].ID)
	}
}

func TestIntegration_Template_Search_InnerProduct(t *testing.T) {
	db := testutil.SqlDB(t)
	ctx := context.Background()
	table := testutil.UniqueTable("ai_ip")

	tmpl := ai.NewVectorSearchTemplate(db)
	if err := tmpl.InitSchema(ctx, table, "embedding", 3); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	for _, v := range []struct {
		id  string
		emb []float32
	}{
		{"x", []float32{1, 0, 0}},
		{"y", []float32{0, 1, 0}},
		{"z", []float32{0, 0, 1}},
	} {
		if err := tmpl.StoreEmbedding(ctx, ai.StoreOptions{
			Table: table, IDColumn: "id", VectorColumn: "embedding",
			ID: v.id, Embedding: v.emb,
		}); err != nil {
			t.Fatalf("StoreEmbedding failed: %v", err)
		}
	}

	results, err := tmpl.Search(ctx, ai.SearchOptions{
		Table:        table,
		VectorColumn: "embedding",
		QueryVector:  []float32{1, 0, 0},
		Metric:       ai.InnerProduct,
		Limit:        3,
		Columns:      []string{"id"},
	})
	if err != nil {
		t.Fatalf("Search InnerProduct failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search InnerProduct returned no results")
	}
	// Inner product closest to [1,0,0] should be "x".
	if results[0].ID != "x" {
		t.Errorf("InnerProduct: expected closest to be 'x', got %q", results[0].ID)
	}
}

// --- VectorStore tests ---

func TestIntegration_VectorStore_AddDocuments(t *testing.T) {
	db := testutil.SqlDB(t)
	ctx := context.Background()
	table := testutil.UniqueTable("ai_vs_add")
	embedder := &mockEmbedder{dims: 4}

	store := ai.NewVectorStore(db, embedder,
		ai.WithTableName(table),
		ai.WithInitializeSchema(true),
		ai.WithDimensions(4),
	)

	docs := []ai.VectorDocument{
		{ID: "doc-1", Text: "CockroachDB is distributed SQL"},
		{ID: "doc-2", Text: "Vector search enables semantic queries"},
		{ID: "doc-3", Text: "Go is a compiled language"},
	}

	if err := store.AddDocuments(ctx, docs); err != nil {
		t.Fatalf("AddDocuments failed: %v", err)
	}

	// Verify documents exist.
	var count int
	err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count)
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 documents, got %d", count)
	}
}

func TestIntegration_VectorStore_SimilaritySearch(t *testing.T) {
	db := testutil.SqlDB(t)
	ctx := context.Background()
	table := testutil.UniqueTable("ai_vs_search")
	embedder := &mockEmbedder{dims: 4}

	store := ai.NewVectorStore(db, embedder,
		ai.WithTableName(table),
		ai.WithInitializeSchema(true),
		ai.WithDimensions(4),
		ai.WithDistanceMetric(ai.Cosine),
	)

	docs := []ai.VectorDocument{
		{ID: "doc-db", Text: "CockroachDB distributed database"},
		{ID: "doc-vec", Text: "vector search similarity"},
		{ID: "doc-go", Text: "Go programming language"},
	}

	if err := store.AddDocuments(ctx, docs); err != nil {
		t.Fatalf("AddDocuments failed: %v", err)
	}

	// Search for something semantically related.
	results, err := store.SimilaritySearch(ctx, "CockroachDB distributed database",
		ai.WithTopK(3),
	)
	if err != nil {
		t.Fatalf("SimilaritySearch failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("SimilaritySearch returned no results")
	}

	// With the mock embedder, the same text should produce the same vector,
	// so "doc-db" should be the top result (distance 0).
	if results[0].ID != "doc-db" {
		t.Errorf("expected top result to be doc-db, got %q", results[0].ID)
	}
}

func TestIntegration_VectorStore_DeleteDocuments(t *testing.T) {
	db := testutil.SqlDB(t)
	ctx := context.Background()
	table := testutil.UniqueTable("ai_vs_del")
	embedder := &mockEmbedder{dims: 4}

	store := ai.NewVectorStore(db, embedder,
		ai.WithTableName(table),
		ai.WithInitializeSchema(true),
		ai.WithDimensions(4),
	)

	docs := []ai.VectorDocument{
		{ID: "del-1", Text: "first document"},
		{ID: "del-2", Text: "second document"},
		{ID: "del-3", Text: "third document"},
	}

	if err := store.AddDocuments(ctx, docs); err != nil {
		t.Fatalf("AddDocuments failed: %v", err)
	}

	// Delete two documents.
	if err := store.DeleteDocuments(ctx, []string{"del-1", "del-3"}); err != nil {
		t.Fatalf("DeleteDocuments failed: %v", err)
	}

	// Verify only one remains.
	var count int
	err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count)
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 remaining document, got %d", count)
	}

	// Verify the correct one remains.
	var id string
	err = db.QueryRowContext(ctx, "SELECT id FROM "+table).Scan(&id)
	if err != nil {
		t.Fatalf("select remaining: %v", err)
	}
	if id != "del-2" {
		t.Errorf("expected remaining doc to be del-2, got %q", id)
	}
}

func TestIntegration_VectorStore_AddWithPrecomputedEmbeddings(t *testing.T) {
	db := testutil.SqlDB(t)
	ctx := context.Background()
	table := testutil.UniqueTable("ai_vs_precomp")
	embedder := &mockEmbedder{dims: 3}

	store := ai.NewVectorStore(db, embedder,
		ai.WithTableName(table),
		ai.WithInitializeSchema(true),
		ai.WithDimensions(3),
	)

	// Add documents with pre-computed embeddings (embedder should not be called).
	docs := []ai.VectorDocument{
		{ID: "pre-1", Text: "has embedding", Embedding: []float32{1, 0, 0}},
		{ID: "pre-2", Text: "also has", Embedding: []float32{0, 1, 0}},
	}

	if err := store.AddDocuments(ctx, docs); err != nil {
		t.Fatalf("AddDocuments with pre-computed embeddings failed: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 docs, got %d", count)
	}
}

func TestIntegration_Template_StoreWithMetadata(t *testing.T) {
	db := testutil.SqlDB(t)
	ctx := context.Background()
	table := testutil.UniqueTable("ai_meta")

	tmpl := ai.NewVectorSearchTemplate(db)
	if err := tmpl.InitSchema(ctx, table, "embedding", 3); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Add a content column to support metadata storage.
	_, err := db.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN IF NOT EXISTS category TEXT")
	if err != nil {
		t.Fatalf("add category column: %v", err)
	}

	err = tmpl.StoreEmbedding(ctx, ai.StoreOptions{
		Table:        table,
		IDColumn:     "id",
		VectorColumn: "embedding",
		ID:           "meta-1",
		Embedding:    []float32{0.5, 0.5, 0},
		Metadata:     map[string]any{"category": "test"},
	})
	if err != nil {
		t.Fatalf("StoreEmbedding with metadata failed: %v", err)
	}

	// Verify the metadata column was set.
	var category string
	err = db.QueryRowContext(ctx, "SELECT category FROM "+table+" WHERE id = $1", "meta-1").Scan(&category)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if category != "test" {
		t.Errorf("expected category 'test', got %q", category)
	}
}
