package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewRagChatClient_Defaults(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	store := NewVectorStore(db, emb)
	provider := &mockChatProvider{response: "ok"}

	client := NewRagChatClient(store, provider)
	if client.maxContextDocs != 5 {
		t.Errorf("maxContextDocs = %d, want 5", client.maxContextDocs)
	}
	if client.similarityThreshold != 0.7 {
		t.Errorf("similarityThreshold = %f, want 0.7", client.similarityThreshold)
	}
	if client.systemPrompt != defaultSystemPrompt {
		t.Errorf("systemPrompt = %q", client.systemPrompt)
	}
}

func TestNewRagChatClient_Options(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	store := NewVectorStore(db, emb)
	provider := &mockChatProvider{response: "ok"}
	m := NewInMemoryAIMetrics()

	client := NewRagChatClient(store, provider,
		WithRagMetrics(m),
		WithSystemPrompt("Custom prompt"),
		WithMaxContextDocs(3),
		WithRagSimilarityThreshold(0.8),
	)
	if client.maxContextDocs != 3 {
		t.Errorf("maxContextDocs = %d, want 3", client.maxContextDocs)
	}
	if client.similarityThreshold != 0.8 {
		t.Errorf("similarityThreshold = %f, want 0.8", client.similarityThreshold)
	}
	if client.systemPrompt != "Custom prompt" {
		t.Errorf("systemPrompt = %q", client.systemPrompt)
	}
}

func TestComplete_BuildsMessages(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	store := NewVectorStore(db, emb)
	provider := &mockChatProvider{response: "The answer is 42"}

	client := NewRagChatClient(store, provider)
	resp, err := client.Complete(context.Background(), "What is the meaning of life?")
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if resp.Content != "The answer is 42" {
		t.Errorf("Content = %q, want %q", resp.Content, "The answer is 42")
	}

	if len(provider.messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(provider.messages))
	}
	if provider.messages[0].Role != RoleSystem {
		t.Errorf("first message role = %v, want system", provider.messages[0].Role)
	}
	last := provider.messages[len(provider.messages)-1]
	if last.Role != RoleUser {
		t.Errorf("last message role = %v, want user", last.Role)
	}
	if last.Content != "What is the meaning of life?" {
		t.Errorf("last message content = %q", last.Content)
	}
}

func TestComplete_WithHistory(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	store := NewVectorStore(db, emb)
	provider := &mockChatProvider{response: "continued answer"}

	client := NewRagChatClient(store, provider)
	history := []ChatMessage{
		{Role: RoleUser, Content: "Tell me about Go"},
		{Role: RoleAssistant, Content: "Go is a language by Google"},
	}
	resp, err := client.Complete(context.Background(), "More details?",
		WithHistory(history),
		WithTemperature(0.5),
		WithMaxTokens(512),
	)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if resp.Content != "continued answer" {
		t.Errorf("Content = %q", resp.Content)
	}

	if len(provider.messages) != 4 {
		t.Fatalf("expected 4 messages (system + 2 history + user), got %d", len(provider.messages))
	}
}

func TestComplete_ProviderError(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	store := NewVectorStore(db, emb)
	provider := &mockChatProvider{err: errors.New("LLM unavailable")}
	m := NewInMemoryAIMetrics()

	client := NewRagChatClient(store, provider, WithRagMetrics(m))
	_, err := client.Complete(context.Background(), "test")
	if err == nil {
		t.Error("expected error from provider")
	}
	if m.ChatCompletionCount() != 1 {
		t.Errorf("chat count = %d, want 1", m.ChatCompletionCount())
	}
}

func TestComplete_MetricsRecorded(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	m := NewInMemoryAIMetrics()
	store := NewVectorStore(db, emb, WithStoreMetrics(m))
	provider := &mockChatProvider{response: "answer"}

	client := NewRagChatClient(store, provider, WithRagMetrics(m))
	_, err := client.Complete(context.Background(), "question")
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if m.ChatCompletionCount() != 1 {
		t.Errorf("chat count = %d, want 1", m.ChatCompletionCount())
	}
	if m.EmbeddingGenerationCount() != 1 {
		t.Errorf("embedding count = %d, want 1 (query embedding)", m.EmbeddingGenerationCount())
	}
	if m.VectorSearchCount() != 1 {
		t.Errorf("search count = %d, want 1", m.VectorSearchCount())
	}
}

func TestStream_Tokens(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	store := NewVectorStore(db, emb)
	provider := &mockChatProvider{response: "ignored"}

	client := NewRagChatClient(store, provider)
	tokenCh, errCh := client.Stream(context.Background(), "test query")

	var tokens []string
	for token := range tokenCh {
		tokens = append(tokens, token)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	got := strings.Join(tokens, "")
	if got != "Hello world" {
		t.Errorf("streamed = %q, want %q", got, "Hello world")
	}
}

func TestStream_ProviderError(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	store := NewVectorStore(db, emb)
	provider := &mockChatProvider{err: errors.New("stream failed")}

	client := NewRagChatClient(store, provider)
	tokenCh, errCh := client.Stream(context.Background(), "test")

	for range tokenCh {
	}
	err := <-errCh
	if err == nil {
		t.Error("expected error from stream")
	}
}

func TestBuildMessages_NoDocuments(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	store := NewVectorStore(db, emb)
	provider := &mockChatProvider{response: "ok"}

	client := NewRagChatClient(store, provider)
	msgs := client.buildMessages("hello", nil, nil)

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if strings.Contains(msgs[0].Content, "Context from the knowledge base") {
		t.Error("should not include context section when no docs")
	}
}

func TestBuildMessages_WithDocuments(t *testing.T) {
	db := getFakeDB(t)
	emb := newMockEmbedder(3)
	store := NewVectorStore(db, emb)
	provider := &mockChatProvider{response: "ok"}

	client := NewRagChatClient(store, provider)
	docs := []VectorDocument{
		{ID: "1", Text: "CockroachDB is distributed"},
		{ID: "2", Text: "Go is fast"},
	}
	msgs := client.buildMessages("tell me about databases", docs, nil)

	if !strings.Contains(msgs[0].Content, "Context from the knowledge base") {
		t.Error("system message should include context section")
	}
	if !strings.Contains(msgs[0].Content, "[Document 1]") {
		t.Error("missing Document 1 label")
	}
	if !strings.Contains(msgs[0].Content, "CockroachDB is distributed") {
		t.Error("missing document text")
	}
}
