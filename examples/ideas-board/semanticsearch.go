package main

import (
	"context"
	"hash/fnv"
	"math"
	"strings"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/ai"
)

// semanticTable is the backing VECTOR table; shared with the integration test.
const semanticTable = "idea_doc"

// semanticDims is the embedding dimensionality. Small and fixed so the demo is
// deterministic and fast.
const semanticDims = 64

// tag::vector-search[]

// hashingEmbedder is a deterministic, dependency-free EmbeddingGenerator: it hashes
// each word of the text into a fixed-size bag-of-words vector and L2-normalizes the
// result. It mirrors the Java demo's HashingEmbeddingProvider — good enough to
// prove semantic search end to end without calling an external embedding API.
type hashingEmbedder struct {
	dims int
}

func (e hashingEmbedder) Dimensions() int { return e.dims }

func (e hashingEmbedder) GenerateEmbedding(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, e.dims)
	for _, word := range strings.Fields(strings.ToLower(text)) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(word))
		vec[h.Sum32()%uint32(e.dims)]++
	}
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm > 0 {
		inv := float32(1 / math.Sqrt(norm))
		for i := range vec {
			vec[i] *= inv
		}
	}
	return vec, nil
}

// SemanticIdeaService runs similarity search over ideas, backed by the ai module's
// VectorStore on a CockroachDB native VECTOR column. Store upserts ideas (embedded
// automatically by the hashing embedder) and FindSimilar returns the nearest ideas
// by cosine distance, closest-first.
//
// It uses brute-force KNN (no CREATE VECTOR INDEX), which needs only the VECTOR
// type and distance operator — available on CockroachDB v25.2+.
type SemanticIdeaService struct {
	store *ai.VectorStore
}

// NewSemanticIdeaService builds the service over db (a *sql.DB satisfies ai.SQLDB).
// The schema is created lazily on first Store/FindSimilar call.
func NewSemanticIdeaService(db ai.SQLDB) *SemanticIdeaService {
	store := ai.NewVectorStore(db, hashingEmbedder{dims: semanticDims},
		ai.WithTableName(semanticTable),
		ai.WithDimensions(semanticDims),
		ai.WithDistanceMetric(ai.Cosine),
		ai.WithInitializeSchema(true),
	)
	return &SemanticIdeaService{store: store}
}

// Store upserts ideas; each idea's text is embedded automatically.
func (s *SemanticIdeaService) Store(ctx context.Context, ideas ...Idea) error {
	docs := make([]ai.VectorDocument, len(ideas))
	for i, idea := range ideas {
		docs[i] = ai.VectorDocument{ID: idea.ID, Text: idea.Text}
	}
	return s.store.AddDocuments(ctx, docs)
}

// FindSimilar returns the k ideas most similar to query, closest-first.
func (s *SemanticIdeaService) FindSimilar(ctx context.Context, query string, k int) ([]ai.VectorDocument, error) {
	return s.store.SimilaritySearch(ctx, query, ai.WithTopK(k))
}

// end::vector-search[]
