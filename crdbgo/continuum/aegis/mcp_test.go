package aegis

import (
	"errors"
	"testing"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/continuum/mcp"
)

// fakeMCP returns scripted CallResults by tool name and records calls.
type fakeMCP struct {
	results map[string]*mcp.CallResult
	calls   []string
	args    []map[string]any
}

func (f *fakeMCP) ListTools() ([]mcp.Tool, error) { return nil, nil }

func (f *fakeMCP) CallTool(name string, args map[string]any) (*mcp.CallResult, error) {
	f.calls = append(f.calls, name)
	f.args = append(f.args, args)
	if r, ok := f.results[name]; ok {
		return r, nil
	}
	return &mcp.CallResult{}, nil
}

func textResult(s string) *mcp.CallResult {
	return &mcp.CallResult{Content: []mcp.Content{{Type: "text", Text: s}}}
}

func TestMCP_FindingsAndRecommendationsParsed(t *testing.T) {
	f := &fakeMCP{results: map[string]*mcp.CallResult{
		"list_findings":        textResult(`[{"id":"f1","severity":"warning","title":"hot range","detail":"d"}]`),
		"list_recommendations": textResult(`[{"id":"r1","title":"split","detail":"d","suggestedAction":"ALTER ... SPLIT"}]`),
	}}
	c := NewMCP(f, MCPToolNames{Findings: "list_findings", Recommendations: "list_recommendations"})

	fs, err := c.Findings()
	if err != nil || len(fs) != 1 || fs[0].ID != "f1" || fs[0].Severity != "warning" {
		t.Fatalf("Findings = %+v, %v", fs, err)
	}
	rs, err := c.Recommendations()
	if err != nil || len(rs) != 1 || rs[0].SuggestedAction == "" {
		t.Fatalf("Recommendations = %+v, %v", rs, err)
	}
}

func TestMCP_EmitSendsArgs(t *testing.T) {
	f := &fakeMCP{results: map[string]*mcp.CallResult{}}
	c := NewMCP(f, MCPToolNames{Emit: "emit_event"})
	err := c.Emit(AppEvent{Kind: "retry", Source: "svc", Attrs: map[string]any{"attempt": 2}, EpochMillis: 99})
	if err != nil {
		t.Fatalf("Emit error: %v", err)
	}
	if len(f.calls) != 1 || f.calls[0] != "emit_event" {
		t.Fatalf("calls = %v, want [emit_event]", f.calls)
	}
	if f.args[0]["kind"] != "retry" || f.args[0]["epochMillis"] != int64(99) {
		t.Errorf("emit args = %+v", f.args[0])
	}
}

func TestMCP_UnconfiguredToolErrors(t *testing.T) {
	c := NewMCP(&fakeMCP{}, MCPToolNames{})
	if _, err := c.Findings(); !errors.Is(err, ErrToolNotConfigured) {
		t.Errorf("Findings err = %v, want ErrToolNotConfigured", err)
	}
	if _, err := c.Recommendations(); !errors.Is(err, ErrToolNotConfigured) {
		t.Errorf("Recommendations err = %v, want ErrToolNotConfigured", err)
	}
	if err := c.Emit(AppEvent{}); !errors.Is(err, ErrToolNotConfigured) {
		t.Errorf("Emit err = %v, want ErrToolNotConfigured", err)
	}
}

func TestMCP_ToolErrorSurfaced(t *testing.T) {
	f := &fakeMCP{results: map[string]*mcp.CallResult{
		"list_findings": {Content: []mcp.Content{{Type: "text", Text: "boom"}}, IsError: true},
	}}
	c := NewMCP(f, MCPToolNames{Findings: "list_findings"})
	if _, err := c.Findings(); err == nil {
		t.Fatal("expected error from IsError result")
	}
}
