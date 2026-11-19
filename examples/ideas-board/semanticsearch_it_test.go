package main

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

// tag::vector-it[]

// TestSemanticSearch_FindSimilarReturnsNearestFirst proves semantic search end to
// end: ideas are embedded into a native VECTOR column and a query returns them
// ordered closest-first by cosine distance.
//
// Vector enablement: the native VECTOR type needs CockroachDB v25.2+, so this runs
// against a v25.2 image (overridable via CRDB_VECTOR_IMAGE). Brute-force KNN (no
// CREATE VECTOR INDEX) needs only the type and the <=> operator — no feature flag.
// If the image genuinely lacks the VECTOR type, the test SKIPS rather than faking a
// pass. Also skips when Docker is unavailable.
func TestSemanticSearch_FindSimilarReturnsNearestFirst(t *testing.T) {
	image := os.Getenv("CRDB_VECTOR_IMAGE")
	if image == "" {
		image = "cockroachdb/cockroach:latest-v25.2"
	}
	cs, cleanup := startContainer(t, image)
	defer cleanup()
	ctx := context.Background()

	db, err := sql.Open("pgx", cs)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	semantic := NewSemanticIdeaService(db)

	// Store four ideas; the hashing embedder embeds each text automatically. Schema
	// (the VECTOR column) is created lazily on this first call.
	err = semantic.Store(ctx,
		Idea{ID: "dark-mode", Text: "Dark mode toggle"},
		Idea{ID: "mobile", Text: "Mobile dark theme"},
		Idea{ID: "search", Text: "Full text search"},
		Idea{ID: "export", Text: "CSV export"},
	)
	if err != nil {
		if mentionsVectorUnsupported(err) {
			t.Skipf("VECTOR type unsupported on image %s: %v", image, err)
		}
		t.Fatalf("Store: %v", err)
	}

	// Query identical to "dark-mode" text — its embedding matches exactly, so it must
	// come back nearest (cosine distance ~0).
	hits, err := semantic.FindSimilar(ctx, "Dark mode toggle", 4)
	if err != nil {
		t.Fatalf("FindSimilar: %v", err)
	}
	if len(hits) != 4 {
		t.Fatalf("got %d hits, want 4", len(hits))
	}
	if hits[0].ID != "dark-mode" {
		t.Fatalf("nearest idea = %q, want dark-mode", hits[0].ID)
	}
	for i := 1; i < len(hits); i++ {
		if hits[i-1].Score < hits[i].Score {
			t.Fatalf("results not ordered closest-first by similarity: %+v", hits)
		}
	}
	t.Logf("nearest: %s (score %.4f)", hits[0].ID, hits[0].Score)
}

// end::vector-it[]

// mentionsVectorUnsupported reports whether err looks like the running image lacks
// the native VECTOR type, so the IT can skip honestly instead of failing.
func mentionsVectorUnsupported(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "vector") &&
		(strings.Contains(msg, "unsupported") ||
			strings.Contains(msg, "unknown") ||
			strings.Contains(msg, "does not exist") ||
			strings.Contains(msg, "type \"vector\"") ||
			strings.Contains(msg, "syntax error"))
}
