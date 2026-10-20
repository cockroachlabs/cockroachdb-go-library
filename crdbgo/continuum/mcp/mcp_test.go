package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew_SelectsSimulatedWhenNoEndpoint(t *testing.T) {
	if _, ok := New(Config{}).(*SimulatedClient); !ok {
		t.Error("New(zero Config) should return a *SimulatedClient")
	}
	if _, ok := New(Config{Enabled: true, Endpoint: "https://x/mcp"}).(*HTTPClient); !ok {
		t.Error("New with endpoint should return an *HTTPClient")
	}
}

func TestSimulated_ListAndCall(t *testing.T) {
	c := NewSimulated()
	tools, err := c.ListTools()
	if err != nil || len(tools) == 0 {
		t.Fatalf("ListTools = %v, %v; want non-empty", tools, err)
	}
	res, err := c.CallTool("ping", nil)
	if err != nil || res.Text() != "pong" {
		t.Fatalf("CallTool(ping) = %q, %v; want pong", res.Text(), err)
	}
	res, _ = c.CallTool("echo", map[string]any{"k": "v"})
	if !strings.Contains(res.Text(), `"k":"v"`) {
		t.Errorf("echo text = %q, want to contain k:v", res.Text())
	}
	if len(c.Calls()) != 2 {
		t.Errorf("recorded calls = %d, want 2", len(c.Calls()))
	}
}

// mcpTestServer scripts JSON-RPC replies by method and records requests.
func mcpTestServer(t *testing.T, sse bool) (*httptest.Server, *[]map[string]any, *[]http.Header) {
	t.Helper()
	var bodies []map[string]any
	var headers []http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(raw, &req)
		bodies = append(bodies, req)
		headers = append(headers, r.Header.Clone())

		// Notifications (no id) get 202.
		if _, hasID := req["id"]; !hasID {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		id := req["id"]
		var result any
		switch req["method"] {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "sess-123")
			result = map[string]any{"protocolVersion": ProtocolVersion, "serverInfo": map[string]any{"name": "test"}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "list_findings", "description": "d"}}}
		case "tools/call":
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "ok"}}}
		}
		env := map[string]any{"jsonrpc": "2.0", "id": id, "result": result}
		payload, _ := json.Marshal(env)
		if sse {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: message\ndata: "))
			_, _ = w.Write(payload)
			_, _ = w.Write([]byte("\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)
	return srv, &bodies, &headers
}

func TestHTTP_InitializeListCall_JSON(t *testing.T) {
	srv, bodies, headers := mcpTestServer(t, false)
	c := NewHTTP(Config{Endpoint: srv.URL, Token: "tok"}, srv.Client())

	tools, err := c.ListTools()
	if err != nil {
		t.Fatalf("ListTools error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "list_findings" {
		t.Fatalf("tools = %+v, want [list_findings]", tools)
	}
	res, err := c.CallTool("list_findings", map[string]any{"cluster": "c1"})
	if err != nil || res.Text() != "ok" {
		t.Fatalf("CallTool = %q, %v; want ok", res.Text(), err)
	}

	// First request is initialize, then notifications/initialized, then list, call.
	methods := make([]string, 0, len(*bodies))
	for _, b := range *bodies {
		methods = append(methods, b["method"].(string))
	}
	want := []string{"initialize", "notifications/initialized", "tools/list", "tools/call"}
	if strings.Join(methods, ",") != strings.Join(want, ",") {
		t.Errorf("methods = %v, want %v", methods, want)
	}
	// Bearer on every request; session + protocol header after initialize.
	for i, h := range *headers {
		if h.Get("Authorization") != "Bearer tok" {
			t.Errorf("req %d missing bearer", i)
		}
		if !strings.Contains(h.Get("Accept"), "text/event-stream") {
			t.Errorf("req %d Accept = %q, want SSE listed", i, h.Get("Accept"))
		}
	}
	// The tools/list request (index 2) must echo the session id.
	if (*headers)[2].Get("Mcp-Session-Id") != "sess-123" {
		t.Errorf("tools/list missing session id, got %q", (*headers)[2].Get("Mcp-Session-Id"))
	}
}

func TestHTTP_SSEResponseParsed(t *testing.T) {
	srv, _, _ := mcpTestServer(t, true)
	c := NewHTTP(Config{Endpoint: srv.URL}, srv.Client())
	res, err := c.CallTool("list_findings", nil)
	if err != nil || res.Text() != "ok" {
		t.Fatalf("SSE CallTool = %q, %v; want ok", res.Text(), err)
	}
}

func TestHTTP_ServerErrorSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(raw, &req)
		if _, hasID := req["id"]; !hasID {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if req["method"] == "initialize" {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"method not found"}}`))
	}))
	t.Cleanup(srv.Close)
	c := NewHTTP(Config{Endpoint: srv.URL}, srv.Client())
	if _, err := c.ListTools(); err == nil || !strings.Contains(err.Error(), "method not found") {
		t.Fatalf("ListTools err = %v, want server error surfaced", err)
	}
}
