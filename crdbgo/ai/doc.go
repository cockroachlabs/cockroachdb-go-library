// Package ai provides vector search, document storage, and RAG (Retrieval-Augmented Generation)
// capabilities for CockroachDB's native VECTOR type.
//
// CockroachDB supports first-class vector columns (VECTOR(dims)) with distance operators
// (<-> for L2, <=> for Cosine, <#> for InnerProduct) and the C-SPANN distributed vector index.
// This package does NOT use pgvector — it targets CockroachDB's native implementation.
//
// Three abstraction layers:
//
//   - VectorSearchTemplate: Low-level SQL operations (search, store, schema init)
//   - VectorStore: High-level document store with auto-embedding via EmbeddingGenerator
//   - RagChatClient: Full RAG pipeline (embed query → retrieve context → LLM completion)
//
// All types use functional options and accept context.Context for cancellation.
package ai
