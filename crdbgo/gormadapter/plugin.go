package gormadapter

import (
	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
	"gorm.io/gorm"
)

const pluginName = "crdbgo:cockroachdb"

// Plugin implements gorm.Plugin for CockroachDB-specific behavior:
// automatic transaction retry, region routing, and metrics recording.
type Plugin struct {
	config  *crdbgo.Config
	metrics crdbgo.MetricsRecorder
}

// PluginOption configures the Plugin beyond the core crdbgo.Options.
type PluginOption func(*Plugin)

// WithMetrics attaches a MetricsRecorder to the GORM plugin.
func WithMetrics(m crdbgo.MetricsRecorder) PluginOption {
	return func(p *Plugin) { p.metrics = m }
}

// NewPlugin creates a CockroachDB GORM plugin with the given options.
func NewPlugin(opts []crdbgo.Option, pluginOpts ...PluginOption) *Plugin {
	p := &Plugin{
		config: crdbgo.NewConfig(opts...),
	}
	for _, opt := range pluginOpts {
		opt(p)
	}
	return p
}

// Name returns the plugin name for GORM's plugin registry.
func (p *Plugin) Name() string {
	return pluginName
}

// Initialize registers CockroachDB callbacks on the GORM DB instance.
// It hooks into Create, Update, and Delete operations to provide
// automatic retry on serialization errors.
func (p *Plugin) Initialize(db *gorm.DB) error {
	cb := db.Callback()

	// Register retry callbacks for write operations.
	// GORM processes callbacks in registration order within each phase.
	// We register "before" callbacks that set up retry context, and
	// "after" callbacks that handle retry logic.
	if err := cb.Create().Before("gorm:create").Register("crdbgo:before_create", p.beforeCallback); err != nil {
		return err
	}
	if err := cb.Create().After("gorm:create").Register("crdbgo:after_create", p.afterCallback); err != nil {
		return err
	}

	if err := cb.Update().Before("gorm:update").Register("crdbgo:before_update", p.beforeCallback); err != nil {
		return err
	}
	if err := cb.Update().After("gorm:update").Register("crdbgo:after_update", p.afterCallback); err != nil {
		return err
	}

	if err := cb.Delete().Before("gorm:delete").Register("crdbgo:before_delete", p.beforeCallback); err != nil {
		return err
	}
	if err := cb.Delete().After("gorm:delete").Register("crdbgo:after_delete", p.afterCallback); err != nil {
		return err
	}

	return nil
}

// compile-time check that Plugin implements gorm.Plugin.
var _ gorm.Plugin = (*Plugin)(nil)
