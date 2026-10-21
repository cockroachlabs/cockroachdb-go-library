package aegis

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/continuum/mcp"
)

// ErrToolNotConfigured is returned when an Aegis-over-MCP operation is attempted
// without the corresponding tool name configured.
var ErrToolNotConfigured = errors.New("aegis: MCP tool name not configured")

// MCPToolNames maps Aegis operations to the tool names an MCP server advertises.
//
// The Cockroach Continuum MCP tool contract is not pinned in this library: the
// names differ by server and may change. Discover the tools your server exposes
// with [mcp.Client.ListTools] and set the ones you need here. An unset name makes
// the corresponding operation return [ErrToolNotConfigured] rather than guessing.
type MCPToolNames struct {
	Findings        string
	Recommendations string
	Emit            string
}

// MCPClient adapts an [mcp.Client] to the Aegis [Client] interface.
//
// Aegis is reachable read-only over the Cockroach Continuum Cloud MCP server.
// This adapter calls the configured tools and parses their text content as JSON
// into [Finding]/[Recommendation]. It is deliberately tolerant: it decodes the
// tool's text payload with the standard (case-insensitive) JSON mapping.
type MCPClient struct {
	mcp   mcp.Client
	tools MCPToolNames
}

// NewMCP wraps an [mcp.Client] with the given tool-name mapping.
func NewMCP(client mcp.Client, tools MCPToolNames) *MCPClient {
	return &MCPClient{mcp: client, tools: tools}
}

// Findings calls the configured findings tool and decodes its text as a JSON
// array of findings.
func (c *MCPClient) Findings() ([]Finding, error) {
	var out []Finding
	if err := c.readList(c.tools.Findings, "findings", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Recommendations calls the configured recommendations tool and decodes its text
// as a JSON array of recommendations.
func (c *MCPClient) Recommendations() ([]Recommendation, error) {
	var out []Recommendation
	if err := c.readList(c.tools.Recommendations, "recommendations", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Emit calls the configured emit tool with the event fields as arguments.
func (c *MCPClient) Emit(event AppEvent) error {
	if c.tools.Emit == "" {
		return fmt.Errorf("%w: emit", ErrToolNotConfigured)
	}
	args := map[string]any{
		"kind":        event.Kind,
		"source":      event.Source,
		"attrs":       event.Attrs,
		"epochMillis": event.EpochMillis,
	}
	res, err := c.mcp.CallTool(c.tools.Emit, args)
	if err != nil {
		return err
	}
	if res.IsError {
		return fmt.Errorf("aegis: emit tool reported error: %s", res.Text())
	}
	return nil
}

func (c *MCPClient) readList(tool, what string, v any) error {
	if tool == "" {
		return fmt.Errorf("%w: %s", ErrToolNotConfigured, what)
	}
	res, err := c.mcp.CallTool(tool, nil)
	if err != nil {
		return err
	}
	if res.IsError {
		return fmt.Errorf("aegis: %s tool reported error: %s", what, res.Text())
	}
	text := res.Text()
	if text == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(text), v); err != nil {
		return fmt.Errorf("aegis: decode %s: %w", what, err)
	}
	return nil
}

// compile-time interface check
var _ Client = (*MCPClient)(nil)
