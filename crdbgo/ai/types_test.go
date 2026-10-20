package ai

import (
	"strings"
	"testing"
)

func TestToVectorLiteral(t *testing.T) {
	tests := []struct {
		name  string
		input []float32
		want  string
	}{
		{"empty", nil, "[]"},
		{"single", []float32{1.5}, "[1.5]"},
		{"multiple", []float32{0.1, 0.2, 0.3}, "[0.1,0.2,0.3]"},
		{"integers", []float32{1, 2, 3}, "[1,2,3]"},
		{"negative", []float32{-0.5, 0.5}, "[-0.5,0.5]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToVectorLiteral(tt.input)
			if got != tt.want {
				t.Errorf("ToVectorLiteral(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestToVectorLiteral_NoSpaces(t *testing.T) {
	v := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	result := ToVectorLiteral(v)
	if strings.Contains(result, " ") {
		t.Errorf("ToVectorLiteral should not contain spaces, got %q", result)
	}
}

func TestVectorSearchResult_Similarity(t *testing.T) {
	r := VectorSearchResult{ID: "test", Distance: 0.2}
	got := r.Similarity(Cosine)
	want := 0.8
	if got != want {
		t.Errorf("Similarity(Cosine) = %f, want %f", got, want)
	}
}

func TestChatRole_String(t *testing.T) {
	tests := []struct {
		role ChatRole
		want string
	}{
		{RoleSystem, "system"},
		{RoleUser, "user"},
		{RoleAssistant, "assistant"},
		{ChatRole(99), "ChatRole(99)"},
	}
	for _, tt := range tests {
		if got := tt.role.String(); got != tt.want {
			t.Errorf("ChatRole(%d).String() = %q, want %q", tt.role, got, tt.want)
		}
	}
}

func TestNewUUID_Format(t *testing.T) {
	id := newUUID()
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Errorf("newUUID() = %q, want 5 dash-separated parts", id)
	}
	if len(id) != 36 {
		t.Errorf("newUUID() length = %d, want 36", len(id))
	}
}

func TestNewUUID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		id := newUUID()
		if seen[id] {
			t.Fatalf("duplicate UUID: %s", id)
		}
		seen[id] = true
	}
}

func TestVectorDocument_Defaults(t *testing.T) {
	doc := VectorDocument{Text: "hello"}
	if doc.ID != "" {
		t.Errorf("expected empty ID, got %q", doc.ID)
	}
	if doc.Metadata != nil {
		t.Error("expected nil metadata")
	}
	if doc.Score != 0 {
		t.Error("expected zero score")
	}
}
