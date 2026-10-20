package ai

import (
	"sync/atomic"
	"time"
)

// InMemoryAIMetrics provides a thread-safe in-memory [AIMetrics] implementation
// using atomic counters. Suitable for testing and development.
type InMemoryAIMetrics struct {
	vectorSearches        atomic.Int64
	vectorSearchFailures  atomic.Int64
	vectorSearchDurationNs atomic.Int64
	vectorSearchResults   atomic.Int64

	embeddingGenerations  atomic.Int64
	embeddingTokens       atomic.Int64

	chatCompletions       atomic.Int64
}

// NewInMemoryAIMetrics creates an in-memory metrics recorder.
func NewInMemoryAIMetrics() *InMemoryAIMetrics {
	return &InMemoryAIMetrics{}
}

// RecordVectorSearch records a vector search with its start time, result count, and outcome.
func (m *InMemoryAIMetrics) RecordVectorSearch(startNanos int64, resultCount int, success bool) {
	m.vectorSearches.Add(1)
	if !success {
		m.vectorSearchFailures.Add(1)
	}
	m.vectorSearchResults.Add(int64(resultCount))
	elapsed := time.Now().UnixNano() - startNanos
	m.vectorSearchDurationNs.Add(elapsed)
}

// RecordEmbeddingGeneration records an embedding generation call.
func (m *InMemoryAIMetrics) RecordEmbeddingGeneration(startNanos int64, tokenCount int, success bool) {
	m.embeddingGenerations.Add(1)
	m.embeddingTokens.Add(int64(tokenCount))
}

// RecordChatCompletion records a chat completion call.
func (m *InMemoryAIMetrics) RecordChatCompletion(startNanos int64, success bool) {
	m.chatCompletions.Add(1)
}

// VectorSearchCount returns the total number of vector searches recorded.
func (m *InMemoryAIMetrics) VectorSearchCount() int64 {
	return m.vectorSearches.Load()
}

// VectorSearchFailureCount returns the number of failed vector searches.
func (m *InMemoryAIMetrics) VectorSearchFailureCount() int64 {
	return m.vectorSearchFailures.Load()
}

// EmbeddingGenerationCount returns the total number of embedding generations.
func (m *InMemoryAIMetrics) EmbeddingGenerationCount() int64 {
	return m.embeddingGenerations.Load()
}

// ChatCompletionCount returns the total number of chat completions.
func (m *InMemoryAIMetrics) ChatCompletionCount() int64 {
	return m.chatCompletions.Load()
}

// AverageVectorSearchDurationMs returns the average vector search duration in milliseconds.
func (m *InMemoryAIMetrics) AverageVectorSearchDurationMs() float64 {
	count := m.vectorSearches.Load()
	if count == 0 {
		return 0
	}
	totalNs := m.vectorSearchDurationNs.Load()
	return float64(totalNs) / float64(count) / 1e6
}

// Snapshot returns a point-in-time snapshot of all metrics.
func (m *InMemoryAIMetrics) Snapshot() AIMetricsSnapshot {
	return AIMetricsSnapshot{
		VectorSearches:       m.vectorSearches.Load(),
		VectorSearchFailures: m.vectorSearchFailures.Load(),
		VectorSearchResults:  m.vectorSearchResults.Load(),
		EmbeddingGenerations: m.embeddingGenerations.Load(),
		EmbeddingTokens:      m.embeddingTokens.Load(),
		ChatCompletions:      m.chatCompletions.Load(),
		AvgSearchDurationMs:  m.AverageVectorSearchDurationMs(),
	}
}

// Reset zeroes all counters.
func (m *InMemoryAIMetrics) Reset() {
	m.vectorSearches.Store(0)
	m.vectorSearchFailures.Store(0)
	m.vectorSearchDurationNs.Store(0)
	m.vectorSearchResults.Store(0)
	m.embeddingGenerations.Store(0)
	m.embeddingTokens.Store(0)
	m.chatCompletions.Store(0)
}

// AIMetricsSnapshot holds a point-in-time view of AI metrics.
type AIMetricsSnapshot struct {
	VectorSearches       int64
	VectorSearchFailures int64
	VectorSearchResults  int64
	EmbeddingGenerations int64
	EmbeddingTokens      int64
	ChatCompletions      int64
	AvgSearchDurationMs  float64
}

// noopAIMetrics is the default zero-cost metrics implementation.
type noopAIMetrics struct{}

func (n *noopAIMetrics) RecordVectorSearch(int64, int, bool)        {}
func (n *noopAIMetrics) RecordEmbeddingGeneration(int64, int, bool) {}
func (n *noopAIMetrics) RecordChatCompletion(int64, bool)           {}
func (n *noopAIMetrics) VectorSearchCount() int64                   { return 0 }
func (n *noopAIMetrics) VectorSearchFailureCount() int64            { return 0 }
func (n *noopAIMetrics) EmbeddingGenerationCount() int64            { return 0 }
func (n *noopAIMetrics) ChatCompletionCount() int64                 { return 0 }
func (n *noopAIMetrics) AverageVectorSearchDurationMs() float64     { return 0 }

var _ AIMetrics = (*InMemoryAIMetrics)(nil)
var _ AIMetrics = (*noopAIMetrics)(nil)
