package main

import (
	"context"
	"testing"
)

// TestHashingEmbedder_DeterministicAndNormalized proves the embedder the semantic
// service relies on is deterministic (same text -> same vector), correctly
// dimensioned, and L2-normalized — all without a database.
func TestHashingEmbedder_DeterministicAndNormalized(t *testing.T) {
	e := hashingEmbedder{dims: semanticDims}
	ctx := context.Background()

	a, err := e.GenerateEmbedding(ctx, "add dark mode")
	if err != nil {
		t.Fatalf("GenerateEmbedding: %v", err)
	}
	b, _ := e.GenerateEmbedding(ctx, "add dark mode")

	if len(a) != semanticDims {
		t.Fatalf("dims = %d, want %d", len(a), semanticDims)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("not deterministic at %d: %v vs %v", i, a[i], b[i])
		}
	}

	var norm float64
	for _, v := range a {
		norm += float64(v) * float64(v)
	}
	if norm < 0.99 || norm > 1.01 {
		t.Fatalf("not L2-normalized: squared norm = %f, want ~1.0", norm)
	}
}

// TestHashingEmbedder_DistinctTextDiffers sanity-checks that different ideas embed
// to different vectors, so similarity search can tell them apart.
func TestHashingEmbedder_DistinctTextDiffers(t *testing.T) {
	e := hashingEmbedder{dims: semanticDims}
	ctx := context.Background()

	a, _ := e.GenerateEmbedding(ctx, "database replication")
	b, _ := e.GenerateEmbedding(ctx, "user interface theme")

	same := true
	for i := range a {
		if a[i] != b[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("distinct texts produced identical embeddings")
	}
}
