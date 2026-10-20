package ai

import (
	"context"
	"errors"
	"testing"
)

func TestNewVectorStore_Defaults(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	store := NewVectorStore(db, emb)

	if store.tableName != "vector_store" {
		t.Errorf("tableName = %q, want %q", store.tableName, "vector_store")
	}
	if store.contentColumn != "content" {
		t.Errorf("contentColumn = %q, want %q", store.contentColumn, "content")
	}
	if store.embeddingColumn != "embedding" {
		t.Errorf("embeddingColumn = %q, want %q", store.embeddingColumn, "embedding")
	}
	if store.metadataColumn != "metadata" {
		t.Errorf("metadataColumn = %q, want %q", store.metadataColumn, "metadata")
	}
	if store.distanceMetric != Cosine {
		t.Errorf("distanceMetric = %v, want Cosine", store.distanceMetric)
	}
	if store.beamSize != 0 {
		t.Errorf("beamSize = %d, want 0", store.beamSize)
	}
}

func TestNewVectorStore_Options(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	m := NewInMemoryAIMetrics()

	store := NewVectorStore(db, emb,
		WithStoreMetrics(m),
		WithTableName("my_docs"),
		WithContentColumn("text"),
		WithEmbeddingColumn("vec"),
		WithMetadataColumn("meta"),
		WithDistanceMetric(L2),
		WithBeamSize(16),
		WithDimensions(384),
		WithInitializeSchema(true),
	)

	if store.tableName != "my_docs" {
		t.Errorf("tableName = %q, want %q", store.tableName, "my_docs")
	}
	if store.contentColumn != "text" {
		t.Errorf("contentColumn = %q", store.contentColumn)
	}
	if store.embeddingColumn != "vec" {
		t.Errorf("embeddingColumn = %q", store.embeddingColumn)
	}
	if store.metadataColumn != "meta" {
		t.Errorf("metadataColumn = %q", store.metadataColumn)
	}
	if store.distanceMetric != L2 {
		t.Errorf("distanceMetric = %v", store.distanceMetric)
	}
	if store.beamSize != 16 {
		t.Errorf("beamSize = %d", store.beamSize)
	}
	if store.dimensions != 384 {
		t.Errorf("dimensions = %d", store.dimensions)
	}
	if !store.initializeSchema {
		t.Error("initializeSchema should be true")
	}
}

func TestAddDocuments_AutoID(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	store := NewVectorStore(db, emb)

	docs := []VectorDocument{
		{Text: "hello world"},
	}
	err := store.AddDocuments(context.Background(), docs)
	if err != nil {
		t.Fatalf("AddDocuments failed: %v", err)
	}
	if docs[0].ID == "" {
		t.Error("expected auto-generated ID")
	}
	if len(docs[0].ID) != 36 {
		t.Errorf("auto ID length = %d, want 36", len(docs[0].ID))
	}
}

func TestAddDocuments_PresetID(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	store := NewVectorStore(db, emb)

	docs := []VectorDocument{
		{ID: "my-doc-1", Text: "hello"},
	}
	err := store.AddDocuments(context.Background(), docs)
	if err != nil {
		t.Fatalf("AddDocuments failed: %v", err)
	}
	if docs[0].ID != "my-doc-1" {
		t.Errorf("ID changed to %q", docs[0].ID)
	}
}

func TestAddDocuments_AutoEmbedding(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	store := NewVectorStore(db, emb)

	docs := []VectorDocument{
		{ID: "doc-1", Text: "test document"},
	}
	err := store.AddDocuments(context.Background(), docs)
	if err != nil {
		t.Fatalf("AddDocuments failed: %v", err)
	}
	if len(docs[0].Embedding) != 3 {
		t.Errorf("embedding length = %d, want 3", len(docs[0].Embedding))
	}
	if emb.callCount() != 1 {
		t.Errorf("embedder calls = %d, want 1", emb.callCount())
	}
}

func TestAddDocuments_PresetEmbedding(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	store := NewVectorStore(db, emb)

	preset := []float32{0.5, 0.6, 0.7}
	docs := []VectorDocument{
		{ID: "doc-1", Text: "test", Embedding: preset},
	}
	err := store.AddDocuments(context.Background(), docs)
	if err != nil {
		t.Fatalf("AddDocuments failed: %v", err)
	}
	if emb.callCount() != 0 {
		t.Errorf("embedder calls = %d, want 0 (preset embedding)", emb.callCount())
	}
}

func TestAddDocuments_EmbedderError(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	emb.err = errors.New("rate limited")
	store := NewVectorStore(db, emb)

	docs := []VectorDocument{
		{ID: "doc-1", Text: "test"},
	}
	err := store.AddDocuments(context.Background(), docs)
	if err == nil {
		t.Error("expected error from embedder")
	}
	if !errors.Is(err, emb.err) {
		t.Errorf("error = %v, want wrapped rate limited", err)
	}
}

func TestAddDocuments_NoEmbedder(t *testing.T) {
	db := getFakeDB(t)
	store := NewVectorStore(db, nil)

	docs := []VectorDocument{
		{ID: "doc-1", Text: "test"},
	}
	err := store.AddDocuments(context.Background(), docs)
	if err == nil {
		t.Error("expected error when no embedder and no preset embedding")
	}
}

func TestAddDocuments_WithMetrics(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	m := NewInMemoryAIMetrics()
	store := NewVectorStore(db, emb, WithStoreMetrics(m))

	docs := []VectorDocument{
		{Text: "doc one"},
		{Text: "doc two"},
	}
	err := store.AddDocuments(context.Background(), docs)
	if err != nil {
		t.Fatalf("AddDocuments failed: %v", err)
	}
	if m.EmbeddingGenerationCount() != 2 {
		t.Errorf("embedding count = %d, want 2", m.EmbeddingGenerationCount())
	}
}

func TestDeleteDocuments_Empty(t *testing.T) {
	db := getFakeDB(t)
	store := NewVectorStore(db, nil)

	err := store.DeleteDocuments(context.Background(), nil)
	if err != nil {
		t.Fatalf("DeleteDocuments with nil should not error: %v", err)
	}
}

func TestDeleteDocuments_Multiple(t *testing.T) {
	db := getFakeDB(t)
	store := NewVectorStore(db, nil)

	err := store.DeleteDocuments(context.Background(), []string{"id-1", "id-2", "id-3"})
	if err != nil {
		t.Fatalf("DeleteDocuments failed: %v", err)
	}
}

func TestSimilaritySearch_NoEmbedder(t *testing.T) {
	db := getFakeDB(t)
	store := NewVectorStore(db, nil)

	_, err := store.SimilaritySearch(context.Background(), "test query")
	if err == nil {
		t.Error("expected error when no embedder")
	}
}

func TestSimilaritySearch_WithOptions(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	m := NewInMemoryAIMetrics()
	store := NewVectorStore(db, emb, WithStoreMetrics(m))

	docs, err := store.SimilaritySearch(context.Background(), "test query",
		WithTopK(5),
		WithSimilarityThreshold(0.5),
		WithFilterExpression("category = 'tech'"),
	)
	if err != nil {
		t.Fatalf("SimilaritySearch failed: %v", err)
	}
	if docs == nil {
		t.Error("expected non-nil docs slice")
	}
	if m.VectorSearchCount() != 1 {
		t.Errorf("search count = %d, want 1", m.VectorSearchCount())
	}
	if m.EmbeddingGenerationCount() != 1 {
		t.Errorf("embedding count = %d, want 1", m.EmbeddingGenerationCount())
	}
}

func TestWithInitializeSchema(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	store := NewVectorStore(db, emb,
		WithInitializeSchema(true),
		WithDimensions(3),
	)

	docs := []VectorDocument{
		{Text: "test"},
	}
	err := store.AddDocuments(context.Background(), docs)
	if err != nil {
		t.Fatalf("AddDocuments with schema init failed: %v", err)
	}
	if !store.initialized {
		t.Error("expected initialized = true after first call")
	}
}

func TestWithCreateVectorIndex(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	store := NewVectorStore(db, emb,
		WithInitializeSchema(true),
		WithCreateVectorIndex(true),
		WithDimensions(3),
	)

	if !store.createVectorIndex {
		t.Error("createVectorIndex should be true")
	}

	docs := []VectorDocument{
		{Text: "test"},
	}
	err := store.AddDocuments(context.Background(), docs)
	if err != nil {
		t.Fatalf("AddDocuments with vector index creation failed: %v", err)
	}
	if !store.initialized {
		t.Error("expected initialized = true after first call")
	}
}

func TestWithPrefixColumns(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	store := NewVectorStore(db, emb,
		WithInitializeSchema(true),
		WithCreateVectorIndex(true),
		WithPrefixColumns("category", "tenant_id"),
		WithDimensions(3),
	)

	if len(store.prefixColumns) != 2 {
		t.Errorf("prefixColumns length = %d, want 2", len(store.prefixColumns))
	}
	if store.prefixColumns[0] != "category" {
		t.Errorf("prefixColumns[0] = %q, want %q", store.prefixColumns[0], "category")
	}
	if store.prefixColumns[1] != "tenant_id" {
		t.Errorf("prefixColumns[1] = %q, want %q", store.prefixColumns[1], "tenant_id")
	}
}

func TestNewVectorStore_VectorIndexOptions(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	m := NewInMemoryAIMetrics()

	store := NewVectorStore(db, emb,
		WithStoreMetrics(m),
		WithTableName("my_docs"),
		WithDistanceMetric(L2),
		WithCreateVectorIndex(true),
		WithPrefixColumns("region"),
		WithDimensions(384),
		WithInitializeSchema(true),
	)

	if !store.createVectorIndex {
		t.Error("createVectorIndex should be true")
	}
	if len(store.prefixColumns) != 1 || store.prefixColumns[0] != "region" {
		t.Errorf("prefixColumns = %v, want [region]", store.prefixColumns)
	}
}

func TestWithInitializeSchema_AutoDimensions(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(768)
	store := NewVectorStore(db, emb,
		WithInitializeSchema(true),
	)

	docs := []VectorDocument{
		{Text: "test"},
	}
	err := store.AddDocuments(context.Background(), docs)
	if err != nil {
		t.Fatalf("AddDocuments failed: %v", err)
	}
}
