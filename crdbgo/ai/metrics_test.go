package ai

import (
	"testing"
	"time"
)

func TestNewInMemoryAIMetrics_Zeros(t *testing.T) {
	m := NewInMemoryAIMetrics()
	if m.VectorSearchCount() != 0 {
		t.Error("expected 0 searches")
	}
	if m.VectorSearchFailureCount() != 0 {
		t.Error("expected 0 failures")
	}
	if m.EmbeddingGenerationCount() != 0 {
		t.Error("expected 0 embeddings")
	}
	if m.ChatCompletionCount() != 0 {
		t.Error("expected 0 completions")
	}
	if m.AverageVectorSearchDurationMs() != 0 {
		t.Error("expected 0 duration")
	}
}

func TestRecordVectorSearch_Success(t *testing.T) {
	m := NewInMemoryAIMetrics()
	start := time.Now().UnixNano()
	m.RecordVectorSearch(start, 5, true)

	if m.VectorSearchCount() != 1 {
		t.Errorf("search count = %d, want 1", m.VectorSearchCount())
	}
	if m.VectorSearchFailureCount() != 0 {
		t.Errorf("failure count = %d, want 0", m.VectorSearchFailureCount())
	}
}

func TestRecordVectorSearch_Failure(t *testing.T) {
	m := NewInMemoryAIMetrics()
	start := time.Now().UnixNano()
	m.RecordVectorSearch(start, 0, false)

	if m.VectorSearchCount() != 1 {
		t.Errorf("search count = %d, want 1", m.VectorSearchCount())
	}
	if m.VectorSearchFailureCount() != 1 {
		t.Errorf("failure count = %d, want 1", m.VectorSearchFailureCount())
	}
}

func TestRecordEmbeddingGeneration(t *testing.T) {
	m := NewInMemoryAIMetrics()
	start := time.Now().UnixNano()
	m.RecordEmbeddingGeneration(start, 100, true)
	m.RecordEmbeddingGeneration(start, 200, true)

	if m.EmbeddingGenerationCount() != 2 {
		t.Errorf("embedding count = %d, want 2", m.EmbeddingGenerationCount())
	}
}

func TestRecordChatCompletion(t *testing.T) {
	m := NewInMemoryAIMetrics()
	start := time.Now().UnixNano()
	m.RecordChatCompletion(start, true)
	m.RecordChatCompletion(start, false)

	if m.ChatCompletionCount() != 2 {
		t.Errorf("chat count = %d, want 2", m.ChatCompletionCount())
	}
}

func TestAverageVectorSearchDurationMs(t *testing.T) {
	m := NewInMemoryAIMetrics()
	start := time.Now().Add(-10 * time.Millisecond).UnixNano()
	m.RecordVectorSearch(start, 3, true)

	avg := m.AverageVectorSearchDurationMs()
	if avg < 5 {
		t.Errorf("average duration = %f ms, expected >= 5", avg)
	}
}

func TestSnapshot(t *testing.T) {
	m := NewInMemoryAIMetrics()
	start := time.Now().UnixNano()
	m.RecordVectorSearch(start, 5, true)
	m.RecordVectorSearch(start, 0, false)
	m.RecordEmbeddingGeneration(start, 100, true)
	m.RecordChatCompletion(start, true)

	snap := m.Snapshot()
	if snap.VectorSearches != 2 {
		t.Errorf("snap.VectorSearches = %d, want 2", snap.VectorSearches)
	}
	if snap.VectorSearchFailures != 1 {
		t.Errorf("snap.VectorSearchFailures = %d, want 1", snap.VectorSearchFailures)
	}
	if snap.VectorSearchResults != 5 {
		t.Errorf("snap.VectorSearchResults = %d, want 5", snap.VectorSearchResults)
	}
	if snap.EmbeddingGenerations != 1 {
		t.Errorf("snap.EmbeddingGenerations = %d, want 1", snap.EmbeddingGenerations)
	}
	if snap.ChatCompletions != 1 {
		t.Errorf("snap.ChatCompletions = %d, want 1", snap.ChatCompletions)
	}
}

func TestReset(t *testing.T) {
	m := NewInMemoryAIMetrics()
	start := time.Now().UnixNano()
	m.RecordVectorSearch(start, 5, true)
	m.RecordEmbeddingGeneration(start, 100, true)
	m.RecordChatCompletion(start, true)

	m.Reset()

	if m.VectorSearchCount() != 0 {
		t.Error("expected 0 after reset")
	}
	if m.EmbeddingGenerationCount() != 0 {
		t.Error("expected 0 after reset")
	}
	if m.ChatCompletionCount() != 0 {
		t.Error("expected 0 after reset")
	}
}

func TestNoopAIMetrics_Satisfies_Interface(t *testing.T) {
	var m AIMetrics = &noopAIMetrics{}
	m.RecordVectorSearch(0, 0, true)
	m.RecordEmbeddingGeneration(0, 0, true)
	m.RecordChatCompletion(0, true)
	if m.VectorSearchCount() != 0 {
		t.Error("noop should return 0")
	}
	if m.VectorSearchFailureCount() != 0 {
		t.Error("noop should return 0")
	}
	if m.EmbeddingGenerationCount() != 0 {
		t.Error("noop should return 0")
	}
	if m.ChatCompletionCount() != 0 {
		t.Error("noop should return 0")
	}
	if m.AverageVectorSearchDurationMs() != 0 {
		t.Error("noop should return 0")
	}
}
