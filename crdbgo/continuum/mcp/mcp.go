// Package mcp is a minimal client for the Model Context Protocol (MCP) over the
// Streamable HTTP transport. It implements just enough of the 2025-06-18 spec to
// discover and call tools on a remote MCP server — for example the Cockroach
// Continuum Cloud MCP server: the initialize handshake, tools/list, and
// tools/call.
//
// MCP is a public, versioned protocol, so (unlike the not-yet-public Aegis REST
// API) this client speaks the real wire format. Transport is injectable so the
// client is testable without a live server; a blank endpoint selects an in-memory
// [SimulatedClient] so applications compile and run whether or not their
// organization is enrolled.
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// ProtocolVersion is the MCP revision this client negotiates.
const ProtocolVersion = "2025-06-18"

// Config configures the MCP client.
type Config struct {
	// Enabled reports whether MCP integration is turned on.
	Enabled bool
	// Endpoint is the MCP server URL. A blank endpoint selects the simulated
	// in-memory backend.
	Endpoint string
	// Token is the bearer token (e.g. a Cloud service-account key). May be empty.
	Token string
	// ClientName/ClientVersion identify this client in the initialize handshake.
	// Sensible defaults are used when blank.
	ClientName    string
	ClientVersion string
}

// Tool is a tool advertised by an MCP server.
type Tool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

// Content is one content block of a tool-call result.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// CallResult is the result of a tools/call.
type CallResult struct {
	Content           []Content      `json:"content"`
	StructuredContent map[string]any `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
}

// Text returns the concatenated text of all text content blocks.
func (r *CallResult) Text() string {
	var b strings.Builder
	for _, c := range r.Content {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	return b.String()
}

// Client is a minimal MCP client.
type Client interface {
	// ListTools discovers the tools the server advertises.
	ListTools() ([]Tool, error)
	// CallTool invokes a tool by name with the given arguments.
	CallTool(name string, args map[string]any) (*CallResult, error)
}

// New returns a [SimulatedClient] when no endpoint is configured, otherwise an
// [HTTPClient] bound to http.DefaultClient.
func New(cfg Config) Client {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return NewSimulated()
	}
	return NewHTTP(cfg, http.DefaultClient)
}

// HTTPClient speaks the Streamable HTTP transport. It lazily performs the
// initialize handshake before the first tools call.
type HTTPClient struct {
	endpoint string
	token    string
	name     string
	version  string
	http     *http.Client

	id   atomic.Int64
	mu   sync.Mutex
	sess string // Mcp-Session-Id, assigned by the server at initialize
	init bool
}

// NewHTTP returns an HTTP-backed client. A nil httpClient uses http.DefaultClient.
func NewHTTP(cfg Config, httpClient *http.Client) *HTTPClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	name := cfg.ClientName
	if name == "" {
		name = "cockroachdb-go-library"
	}
	version := cfg.ClientVersion
	if version == "" {
		version = "0.1"
	}
	return &HTTPClient{
		endpoint: cfg.Endpoint,
		token:    cfg.Token,
		name:     name,
		version:  version,
		http:     httpClient,
	}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

func (c *HTTPClient) ensureInit() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.init {
		return nil
	}
	params := map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": c.name, "version": c.version},
	}
	if _, err := c.call("initialize", params); err != nil {
		return err
	}
	// Best-effort: a server that rejects the notification must not block us.
	_ = c.notify("notifications/initialized")
	c.init = true
	return nil
}

// ListTools implements Client.
func (c *HTTPClient) ListTools() ([]Tool, error) {
	if err := c.ensureInit(); err != nil {
		return nil, err
	}
	raw, err := c.call("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("mcp: decode tools/list: %w", err)
	}
	return out.Tools, nil
}

// CallTool implements Client.
func (c *HTTPClient) CallTool(name string, args map[string]any) (*CallResult, error) {
	if err := c.ensureInit(); err != nil {
		return nil, err
	}
	if args == nil {
		args = map[string]any{}
	}
	raw, err := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return nil, err
	}
	var res CallResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("mcp: decode tools/call: %w", err)
	}
	return &res, nil
}

// call sends a JSON-RPC request and returns its result, parsing either a plain
// JSON or an SSE response per the Streamable HTTP transport.
func (c *HTTPClient) call(method string, params any) (json.RawMessage, error) {
	body, _ := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: c.id.Add(1), Method: method, Params: params})
	resp, err := c.post(body)
	if err != nil {
		return nil, err
	}
	var rpc rpcResponse
	if err := json.Unmarshal(resp, &rpc); err != nil {
		return nil, fmt.Errorf("mcp: %s: decode response: %w", method, err)
	}
	if rpc.Error != nil {
		return nil, fmt.Errorf("mcp: %s: server error %d: %s", method, rpc.Error.Code, rpc.Error.Message)
	}
	return rpc.Result, nil
}

// notify sends a JSON-RPC notification (no id, no response body expected).
func (c *HTTPClient) notify(method string) error {
	body, _ := json.Marshal(rpcRequest{JSONRPC: "2.0", Method: method})
	_, err := c.post(body)
	return err
}

func (c *HTTPClient) post(body []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.sess != "" {
		req.Header.Set("Mcp-Session-Id", c.sess)
	}
	if c.init {
		req.Header.Set("MCP-Protocol-Version", ProtocolVersion)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.sess = sid
	}
	// 202 Accepted with no body is the valid reply to a notification.
	if resp.StatusCode == http.StatusAccepted {
		return nil, nil
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mcp: HTTP %d: %s", resp.StatusCode, truncate(buf.String(), 200))
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return extractSSEData(buf.Bytes()), nil
	}
	return buf.Bytes(), nil
}

// extractSSEData concatenates the `data:` payloads of an SSE body. For a single
// JSON-RPC response this yields the one message object.
func extractSSEData(body []byte) []byte {
	var out bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if after, ok := strings.CutPrefix(line, "data:"); ok {
			out.WriteString(strings.TrimSpace(after))
		}
	}
	return out.Bytes()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
