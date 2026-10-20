package ai

import (
	"crypto/rand"
	"fmt"
)

// VectorSearchResult holds a single result from a vector similarity search
// against CockroachDB's native VECTOR type.
type VectorSearchResult struct {
	ID       string
	Distance float64
	Metadata map[string]any
}

// Similarity returns the distance converted to a [0,1] similarity score
// using the given metric.
func (r VectorSearchResult) Similarity(metric DistanceMetric) float64 {
	return metric.ToSimilarity(r.Distance)
}

// VectorDocument represents a document stored in a vector store.
// Text is embedded automatically when added via VectorStore.
type VectorDocument struct {
	ID        string
	Text      string
	Metadata  map[string]any
	Embedding []float32
	Score     float64
}

// ChatRole identifies the role of a message in a chat conversation (system,
// user, or assistant).
type ChatRole int

const (
	// RoleSystem is the system prompt that sets the AI's behavior.
	RoleSystem ChatRole = iota
	// RoleUser is a message from the user/human.
	RoleUser
	// RoleAssistant is a message from the AI/LLM.
	RoleAssistant
)

var roleNames = [...]string{"system", "user", "assistant"}

// String returns the lowercase role name ("system", "user", "assistant").
func (r ChatRole) String() string {
	if r < 0 || int(r) >= len(roleNames) {
		return fmt.Sprintf("ChatRole(%d)", r)
	}
	return roleNames[r]
}

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	Role    ChatRole
	Content string
}

// ChatResponse holds the result of a chat completion.
type ChatResponse struct {
	Content         string
	SourceDocuments []VectorDocument
	TokensUsed      int
}

func newUUID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ToVectorLiteral converts a float32 slice to CockroachDB's vector literal format.
// Output: "[0.1,0.2,0.3]" (square brackets, comma-separated, no spaces).
func ToVectorLiteral(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	buf := make([]byte, 0, len(v)*8+2)
	buf = append(buf, '[')
	for i, f := range v {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = fmt.Appendf(buf, "%g", f)
	}
	buf = append(buf, ']')
	return string(buf)
}
