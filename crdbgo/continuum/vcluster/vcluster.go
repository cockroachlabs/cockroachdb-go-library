// Package vcluster provides Cockroach vCluster workload tagging for CockroachDB.
//
// A Cockroach vCluster (virtual cluster) isolates and prioritizes workloads that
// share physical capacity. This package binds a vCluster name and an optional
// workload priority into a [context.Context] — the same mechanism the core
// library uses for region routing — so that a transaction run with that context
// is tagged at the session level with an application_name and the mapped
// transaction quality-of-service (QoS).
//
// Scope (v1): this performs workload tagging and QoS assignment only. Routing a
// workload to a different vCluster endpoint is platform-dependent and is
// intentionally not simulated here — that is future work.
//
// Typical use:
//
//	ctx = vcluster.With(ctx, "analytics", vcluster.Low)
//	err = pool.ExecuteTx(ctx, func(tx pgx.Tx) error { ... })
//
// or with functional options:
//
//	ctx = vcluster.WithContext(ctx,
//	    vcluster.WithVirtualCluster("analytics"),
//	    vcluster.WithWorkload(vcluster.Low))
package vcluster

import "context"

// Priority is the relative importance of a workload sharing a virtual cluster.
// It maps to CockroachDB's default_transaction_quality_of_service session
// variable via [Priority.QoS].
type Priority int

const (
	// Normal maps to QoS "regular" (the CockroachDB default). It is the zero
	// value so an unset priority behaves like the cluster default.
	Normal Priority = iota

	// High maps to QoS "critical" — latency-sensitive OLTP work.
	High

	// Low maps to QoS "background" — analytics or batch work that should yield
	// to higher-priority workloads sharing the same capacity.
	Low
)

// QoS returns the CockroachDB default_transaction_quality_of_service value for
// this priority: one of "critical", "regular", or "background".
func (p Priority) QoS() string {
	switch p {
	case High:
		return "critical"
	case Low:
		return "background"
	default:
		return "regular"
	}
}

// String returns a human-readable name for the priority.
func (p Priority) String() string {
	switch p {
	case High:
		return "High"
	case Low:
		return "Low"
	default:
		return "Normal"
	}
}

// settings is the resolved vCluster state carried in a context.
type settings struct {
	name        string
	priority    Priority
	hasPriority bool
}

type contextKey struct{}

// Option configures the vCluster [settings] applied to a context by
// [WithContext].
type Option func(*settings)

// WithVirtualCluster tags work with the named virtual cluster. The name is
// applied as the session application_name at transaction start.
func WithVirtualCluster(name string) Option {
	return func(s *settings) { s.name = name }
}

// WithWorkload tags work with a workload priority, applied as the transaction
// quality-of-service at transaction start.
func WithWorkload(p Priority) Option {
	return func(s *settings) {
		s.priority = p
		s.hasPriority = true
	}
}

// WithContext returns a copy of ctx carrying the vCluster tagging described by
// opts. Options merge onto any vCluster settings already present in ctx, so
// successive calls compose (e.g. set the cluster in one call, the workload in
// another).
func WithContext(ctx context.Context, opts ...Option) context.Context {
	var s settings
	if existing, ok := ctx.Value(contextKey{}).(*settings); ok {
		s = *existing
	}
	for _, opt := range opts {
		opt(&s)
	}
	return context.WithValue(ctx, contextKey{}, &s)
}

// With is a convenience wrapper that tags ctx with both a virtual cluster name
// and a workload priority in a single call.
func With(ctx context.Context, name string, priority Priority) context.Context {
	return WithContext(ctx, WithVirtualCluster(name), WithWorkload(priority))
}

// Tag is the resolved vCluster tagging read from a context by [FromContext].
// A blank field means the corresponding session setting should be left at its
// current value.
type Tag struct {
	// VirtualCluster is the application_name to apply, or "" if unset.
	VirtualCluster string
	// QoS is the default_transaction_quality_of_service value to apply
	// ("critical"/"regular"/"background"), or "" if no workload was set.
	QoS string
}

// FromContext extracts the vCluster [Tag] from ctx. The second return value
// reports whether any vCluster tagging was attached. Adapters use this to apply
// session tags at the start of a transaction.
func FromContext(ctx context.Context) (Tag, bool) {
	s, ok := ctx.Value(contextKey{}).(*settings)
	if !ok {
		return Tag{}, false
	}
	tag := Tag{VirtualCluster: s.name}
	if s.hasPriority {
		tag.QoS = s.priority.QoS()
	}
	return tag, true
}
