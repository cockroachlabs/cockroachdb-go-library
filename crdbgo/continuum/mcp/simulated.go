package mcp

import (
	"encoding/json"
	"fmt"
	"sync"
)

// SimulatedClient is an in-memory MCP client for development and tests. It
// advertises a small canned tool set and echoes tool calls, so code that drives
// an MCP server compiles and runs offline. It records every call for assertions.
type SimulatedClient struct {
	mu    sync.Mutex
	tools []Tool
	calls []SimCall
}

// SimCall is a recorded tools/call.
type SimCall struct {
	Name string
	Args map[string]any
}

// NewSimulated returns a simulated client with a couple of sample tools.
func NewSimulated() *SimulatedClient {
	return &SimulatedClient{
		tools: []Tool{
			{Name: "ping", Title: "Ping", Description: "Health check; returns pong."},
			{Name: "echo", Title: "Echo", Description: "Echoes its arguments as JSON text."},
		},
	}
}

// ListTools implements Client.
func (s *SimulatedClient) ListTools() ([]Tool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Tool(nil), s.tools...), nil
}

// CallTool implements Client. "ping" returns "pong"; any other tool echoes its
// arguments as JSON text.
func (s *SimulatedClient) CallTool(name string, args map[string]any) (*CallResult, error) {
	s.mu.Lock()
	s.calls = append(s.calls, SimCall{Name: name, Args: args})
	s.mu.Unlock()

	if name == "ping" {
		return &CallResult{Content: []Content{{Type: "text", Text: "pong"}}}, nil
	}
	b, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("mcp: simulated marshal: %w", err)
	}
	return &CallResult{Content: []Content{{Type: "text", Text: string(b)}}}, nil
}

// Calls returns the recorded tool calls.
func (s *SimulatedClient) Calls() []SimCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SimCall(nil), s.calls...)
}
