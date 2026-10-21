// Package agent provides a minimal, dependency-light MCP-style tool surface over
// CockroachDB library operations, the "Agent/MCP" surface of Cockroach Continuum.
//
// Tools are registered explicitly (Go has no annotations) with a name, a
// description, a handler, and a destructive flag. The [Registry] then exposes an
// MCP-style surface: [Registry.ListTools] returns the tools/list shape and
// [Registry.Invoke] runs a tool (tools/call). It is least-privilege by default —
// destructive tools are hidden from listing and rejected on invocation unless
// [Registry.AllowDestructive] is set.
//
// A network transport (stdio/HTTP) is intentionally out of scope for this
// increment; ListTools/Invoke return values a transport can relay.
package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/continuum/plenum"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ToolFunc is the handler for an agent tool. It receives the decoded arguments
// (never nil) and returns a JSON-serializable result or an error.
type ToolFunc func(args map[string]any) (any, error)

// tool is an internal registered tool.
type tool struct {
	name        string
	description string
	destructive bool
	fn          ToolFunc
}

// ToolDescriptor is the MCP tools/list description of a single tool.
type ToolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Registry holds the set of agent-callable tools and enforces least-privilege
// access to destructive ones. The zero value is not usable; create one with
// [NewRegistry].
type Registry struct {
	mu               sync.Mutex
	tools            map[string]*tool
	order            []string
	allowDestructive bool
}

// NewRegistry creates an empty tool [Registry].
func NewRegistry() *Registry {
	return &Registry{tools: map[string]*tool{}}
}

// Register adds a tool to the registry. Registering an existing name overwrites
// it. It returns the registry for chaining.
func (r *Registry) Register(name, description string, fn ToolFunc, destructive bool) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = &tool{name: name, description: description, destructive: destructive, fn: fn}
	return r
}

// AllowDestructive controls whether destructive tools are listed and invokable.
// It returns the registry for chaining.
func (r *Registry) AllowDestructive(allow bool) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.allowDestructive = allow
	return r
}

// ListTools returns the descriptors for all callable tools in registration
// order (MCP tools/list shape). Destructive tools are omitted unless
// [Registry.AllowDestructive] has been enabled.
func (r *Registry) ListTools() []ToolDescriptor {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ToolDescriptor, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		if t.destructive && !r.allowDestructive {
			continue
		}
		out = append(out, ToolDescriptor{
			Name:        t.name,
			Description: t.description,
			InputSchema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": true,
			},
		})
	}
	return out
}

// Invoke runs the named tool with args (MCP tools/call shape). It returns an
// error if the tool is unknown, or if it is destructive and destructive tools
// are not allowed. A nil args map is treated as empty.
func (r *Registry) Invoke(name string, args map[string]any) (any, error) {
	r.mu.Lock()
	t, ok := r.tools[name]
	allowDestructive := r.allowDestructive
	r.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("agent: unknown tool %q", name)
	}
	if t.destructive && !allowDestructive {
		return nil, fmt.Errorf("agent: tool %q is destructive and not allowed", name)
	}
	if args == nil {
		args = map[string]any{}
	}
	return t.fn(args)
}

// opsTimeout bounds each built-in ops tool invocation.
const opsTimeout = 5 * time.Second

// OpsTools returns a [Registry] preloaded with safe, read-only CockroachDB
// operations backed by pool: a "health" check and a Cockroach Plenum
// "storage_summary". Both are non-destructive. Register additional tools on the
// returned registry as needed.
func OpsTools(pool *pgxpool.Pool) *Registry {
	r := NewRegistry()

	r.Register("health",
		"Ping the CockroachDB cluster and report reachability.",
		func(map[string]any) (any, error) {
			ctx, cancel := context.WithTimeout(context.Background(), opsTimeout)
			defer cancel()
			if err := pool.Ping(ctx); err != nil {
				return map[string]any{"status": "unreachable", "error": err.Error()}, nil
			}
			return map[string]any{"status": "ok"}, nil
		}, false)

	r.Register("storage_summary",
		"Cockroach Plenum per-table storage summary (rows and approximate bytes) for the current database.",
		func(map[string]any) (any, error) {
			ctx, cancel := context.WithTimeout(context.Background(), opsTimeout)
			defer cancel()
			tables, err := plenum.Summary(ctx, pool)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(tables))
			for _, t := range tables {
				row := map[string]any{"schema": t.Schema, "table": t.Table, "rows": nil, "bytes": nil}
				if t.Rows != nil {
					row["rows"] = *t.Rows
				}
				if t.Bytes != nil {
					row["bytes"] = *t.Bytes
				}
				out = append(out, row)
			}
			return out, nil
		}, false)

	return r
}
