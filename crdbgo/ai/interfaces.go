package ai

import (
	"context"
	"database/sql"
)

// EmbeddingGenerator converts text into vector embeddings for storage in
// CockroachDB's native VECTOR columns. Implementations wrap specific providers
// (OpenAI, Ollama, etc.). The [Dimensions] method reports the output vector
// size, which must match the VECTOR(dims) column definition.
type EmbeddingGenerator interface {
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)
	Dimensions() int
}

// ChatCompletionProvider generates text responses from a sequence of chat messages.
// Implementations wrap specific LLM providers (OpenAI, Anthropic, Ollama, etc.).
type ChatCompletionProvider interface {
	Complete(ctx context.Context, messages []ChatMessage, temperature float64, maxTokens int) (string, error)
	Stream(ctx context.Context, messages []ChatMessage, temperature float64, maxTokens int) (<-chan string, <-chan error)
}

// AIMetrics records performance metrics for AI operations including vector
// search, embedding generation, and chat completion latency/success.
type AIMetrics interface {
	RecordVectorSearch(startNanos int64, resultCount int, success bool)
	RecordEmbeddingGeneration(startNanos int64, tokenCount int, success bool)
	RecordChatCompletion(startNanos int64, success bool)

	VectorSearchCount() int64
	VectorSearchFailureCount() int64
	EmbeddingGenerationCount() int64
	ChatCompletionCount() int64
	AverageVectorSearchDurationMs() float64
}

// SQLDB abstracts the database operations needed by the AI package.
// [database/sql.DB] satisfies this interface directly; pass it to
// [NewVectorSearchTemplate] or [NewVectorStore].
type SQLDB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}
