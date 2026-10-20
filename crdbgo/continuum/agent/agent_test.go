package agent

import (
	"errors"
	"testing"
)

func names(descs []ToolDescriptor) map[string]bool {
	m := map[string]bool{}
	for _, d := range descs {
		m[d.Name] = true
	}
	return m
}

func TestRegistry_ListAndInvoke(t *testing.T) {
	r := NewRegistry().
		Register("echo", "echoes the msg argument", func(args map[string]any) (any, error) {
			return args["msg"], nil
		}, false)

	descs := r.ListTools()
	if len(descs) != 1 || descs[0].Name != "echo" {
		t.Fatalf("ListTools = %+v, want single echo tool", descs)
	}
	// MCP tools/list shape: inputSchema is an object.
	if descs[0].InputSchema["type"] != "object" {
		t.Errorf("inputSchema.type = %v, want object", descs[0].InputSchema["type"])
	}

	got, err := r.Invoke("echo", map[string]any{"msg": "hi"})
	if err != nil {
		t.Fatalf("Invoke echo: %v", err)
	}
	if got != "hi" {
		t.Errorf("Invoke echo = %v, want hi", got)
	}
}

func TestRegistry_UnknownTool(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Invoke("nope", nil); err == nil {
		t.Fatal("expected error invoking unknown tool")
	}
}

func TestRegistry_DestructiveGating(t *testing.T) {
	r := NewRegistry().
		Register("safe", "safe op", func(map[string]any) (any, error) { return "ok", nil }, false).
		Register("drop_it", "destructive op", func(map[string]any) (any, error) { return "dropped", nil }, true)

	// Hidden by default.
	if names(r.ListTools())["drop_it"] {
		t.Error("destructive tool should be hidden by default")
	}
	if _, err := r.Invoke("drop_it", nil); err == nil {
		t.Error("destructive tool should be rejected by default")
	}

	// Visible + invokable once allowed.
	r.AllowDestructive(true)
	if !names(r.ListTools())["drop_it"] {
		t.Error("destructive tool should be listed once allowed")
	}
	got, err := r.Invoke("drop_it", nil)
	if err != nil {
		t.Fatalf("Invoke drop_it after allow: %v", err)
	}
	if got != "dropped" {
		t.Errorf("Invoke drop_it = %v, want dropped", got)
	}
}

func TestRegistry_InvokePropagatesError(t *testing.T) {
	sentinel := errors.New("boom")
	r := NewRegistry().Register("fail", "always fails",
		func(map[string]any) (any, error) { return nil, sentinel }, false)
	if _, err := r.Invoke("fail", nil); !errors.Is(err, sentinel) {
		t.Errorf("Invoke error = %v, want sentinel", err)
	}
}
