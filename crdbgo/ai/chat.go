package ai

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const defaultSystemPrompt = "You are a helpful assistant. Use the provided context to answer questions accurately."

// RagChatClient implements Retrieval-Augmented Generation (RAG) backed by
// CockroachDB vector search: embed query -> retrieve relevant documents ->
// build context -> LLM completion. It combines a [VectorStore] for retrieval
// with a [ChatCompletionProvider] for generation.
type RagChatClient struct {
	vectorStore     *VectorStore
	provider        ChatCompletionProvider
	metrics         AIMetrics
	systemPrompt    string
	maxContextDocs  int
	similarityThreshold float64
}

// RagOption configures a [RagChatClient]. Pass options to [NewRagChatClient].
type RagOption func(*RagChatClient)

// NewRagChatClient creates a RAG client that combines CockroachDB vector search
// with LLM completion. Defaults: 5 context documents, 0.7 similarity threshold.
func NewRagChatClient(store *VectorStore, provider ChatCompletionProvider, opts ...RagOption) *RagChatClient {
	r := &RagChatClient{
		vectorStore:         store,
		provider:            provider,
		metrics:             &noopAIMetrics{},
		systemPrompt:        defaultSystemPrompt,
		maxContextDocs:      5,
		similarityThreshold: 0.7,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// WithRagMetrics attaches an [AIMetrics] recorder to the RAG client.
func WithRagMetrics(m AIMetrics) RagOption {
	return func(r *RagChatClient) {
		if m != nil {
			r.metrics = m
		}
	}
}

// WithSystemPrompt sets the system prompt prepended to each RAG conversation.
func WithSystemPrompt(prompt string) RagOption {
	return func(r *RagChatClient) { r.systemPrompt = prompt }
}

// WithMaxContextDocs sets the maximum number of documents retrieved from the
// vector store to build the LLM context. Default is 5.
func WithMaxContextDocs(n int) RagOption {
	return func(r *RagChatClient) { r.maxContextDocs = n }
}

// WithRagSimilarityThreshold sets the minimum similarity score for retrieved
// documents. Documents below this threshold are excluded. Default is 0.7.
func WithRagSimilarityThreshold(t float64) RagOption {
	return func(r *RagChatClient) { r.similarityThreshold = t }
}

// Complete performs a one-shot RAG: retrieves context, then gets an LLM response.
func (r *RagChatClient) Complete(ctx context.Context, userMessage string, opts ...ChatOption) (*ChatResponse, error) {
	cfg := chatConfig{
		temperature: 0.7,
		maxTokens:   1024,
	}
	for _, o := range opts {
		o(&cfg)
	}

	startNanos := time.Now().UnixNano()

	docs, err := r.vectorStore.SimilaritySearch(ctx, userMessage,
		WithTopK(r.maxContextDocs),
		WithSimilarityThreshold(r.similarityThreshold),
	)
	if err != nil {
		r.metrics.RecordChatCompletion(startNanos, false)
		return nil, fmt.Errorf("ai: rag retrieve: %w", err)
	}

	messages := r.buildMessages(userMessage, docs, cfg.history)

	content, err := r.provider.Complete(ctx, messages, cfg.temperature, cfg.maxTokens)
	if err != nil {
		r.metrics.RecordChatCompletion(startNanos, false)
		return nil, fmt.Errorf("ai: rag complete: %w", err)
	}

	r.metrics.RecordChatCompletion(startNanos, true)
	return &ChatResponse{
		Content:         content,
		SourceDocuments: docs,
	}, nil
}

// Stream performs RAG with streaming LLM output. Returns a channel of tokens
// and a channel for the final error (nil on success).
func (r *RagChatClient) Stream(ctx context.Context, userMessage string, opts ...ChatOption) (<-chan string, <-chan error) {
	cfg := chatConfig{
		temperature: 0.7,
		maxTokens:   1024,
	}
	for _, o := range opts {
		o(&cfg)
	}

	errCh := make(chan error, 1)
	startNanos := time.Now().UnixNano()

	docs, err := r.vectorStore.SimilaritySearch(ctx, userMessage,
		WithTopK(r.maxContextDocs),
		WithSimilarityThreshold(r.similarityThreshold),
	)
	if err != nil {
		r.metrics.RecordChatCompletion(startNanos, false)
		errCh <- fmt.Errorf("ai: rag retrieve: %w", err)
		close(errCh)
		tokenCh := make(chan string)
		close(tokenCh)
		return tokenCh, errCh
	}

	messages := r.buildMessages(userMessage, docs, cfg.history)
	tokenCh, providerErrCh := r.provider.Stream(ctx, messages, cfg.temperature, cfg.maxTokens)

	outTokenCh := make(chan string, 32)
	go func() {
		defer close(outTokenCh)
		defer close(errCh)
		for token := range tokenCh {
			outTokenCh <- token
		}
		if providerErr := <-providerErrCh; providerErr != nil {
			r.metrics.RecordChatCompletion(startNanos, false)
			errCh <- providerErr
		} else {
			r.metrics.RecordChatCompletion(startNanos, true)
		}
	}()

	return outTokenCh, errCh
}

func (r *RagChatClient) buildMessages(userMessage string, docs []VectorDocument, history []ChatMessage) []ChatMessage {
	var messages []ChatMessage

	systemContent := r.systemPrompt
	if len(docs) > 0 {
		var contextBuilder strings.Builder
		contextBuilder.WriteString(systemContent)
		contextBuilder.WriteString("\n\nContext from the knowledge base:\n")
		for i, doc := range docs {
			fmt.Fprintf(&contextBuilder, "\n[Document %d]\n%s\n", i+1, doc.Text)
		}
		systemContent = contextBuilder.String()
	}
	messages = append(messages, ChatMessage{Role: RoleSystem, Content: systemContent})

	messages = append(messages, history...)
	messages = append(messages, ChatMessage{Role: RoleUser, Content: userMessage})

	return messages
}

// ChatOption configures a single [RagChatClient.Complete] or
// [RagChatClient.Stream] call.
type ChatOption func(*chatConfig)

type chatConfig struct {
	temperature float64
	maxTokens   int
	history     []ChatMessage
}

// WithTemperature sets the LLM sampling temperature. Default is 0.7.
func WithTemperature(t float64) ChatOption {
	return func(c *chatConfig) { c.temperature = t }
}

// WithMaxTokens sets the maximum token count for the LLM response. Default is 1024.
func WithMaxTokens(n int) ChatOption {
	return func(c *chatConfig) { c.maxTokens = n }
}

// WithHistory provides prior conversation messages for multi-turn RAG.
func WithHistory(h []ChatMessage) ChatOption {
	return func(c *chatConfig) { c.history = h }
}
