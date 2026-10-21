package gormadapter

import (
	"context"
	"time"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
	"gorm.io/gorm"
)

// retryContextKey is used to store retry state across before/after callbacks.
type retryContextKey struct{}

// retryState tracks retry metadata for a single GORM operation.
type retryState struct {
	StartTime time.Time
	Attempt   int
}

// beforeCallback runs before each Create/Update/Delete and initializes retry state.
func (p *Plugin) beforeCallback(db *gorm.DB) {
	if db.Statement == nil {
		return
	}

	ctx := db.Statement.Context

	// Only initialize retry state on first attempt (not on retries).
	if ctx.Value(retryContextKey{}) != nil {
		return
	}

	state := &retryState{
		StartTime: time.Now(),
		Attempt:   0,
	}

	// Inject region if configured at plugin level.
	if p.config.Region != "" {
		db.Exec("SET crdb_region = ?", p.config.Region)
	}

	// Check for region in context (overrides plugin-level).
	if rc, ok := crdbgo.RegionFromContext(ctx); ok && rc.Region != "" {
		db.Exec("SET crdb_region = ?", rc.Region)
	}

	db.Statement.Context = context.WithValue(ctx, retryContextKey{}, state)
}

// afterCallback runs after each Create/Update/Delete and handles retry logic.
func (p *Plugin) afterCallback(db *gorm.DB) {
	if db.Error == nil {
		p.recordSuccess(db)
		return
	}

	if !IsRetryable(db.Error) {
		p.recordFailure(db)
		return
	}

	state := retryStateFromContext(db)
	if state == nil {
		return
	}

	if !p.config.Retry.ShouldRetry(state.Attempt) {
		p.recordFailure(db)
		return
	}

	// Calculate and apply backoff delay.
	delay := p.config.Retry.NextDelay(state.Attempt)
	if p.metrics != nil {
		p.metrics.RecordRetry(delay)
	}

	state.Attempt++

	select {
	case <-db.Statement.Context.Done():
		db.Error = db.Statement.Context.Err()
		return
	case <-time.After(delay):
	}

	// Clear the error to allow GORM to re-execute.
	db.Error = nil
}

// recordSuccess records a successful transaction in metrics.
func (p *Plugin) recordSuccess(db *gorm.DB) {
	if p.metrics == nil {
		return
	}
	state := retryStateFromContext(db)
	if state == nil {
		return
	}
	p.metrics.RecordTransaction(time.Since(state.StartTime), state.Attempt, true)
}

// recordFailure records a failed transaction in metrics.
func (p *Plugin) recordFailure(db *gorm.DB) {
	if p.metrics == nil {
		return
	}
	state := retryStateFromContext(db)
	if state == nil {
		return
	}
	p.metrics.RecordTransaction(time.Since(state.StartTime), state.Attempt, false)
}

// retryStateFromContext retrieves retry state from the GORM DB.
func retryStateFromContext(db *gorm.DB) *retryState {
	if db == nil || db.Statement == nil {
		return nil
	}
	v := db.Statement.Context.Value(retryContextKey{})
	if v == nil {
		return nil
	}
	state, ok := v.(*retryState)
	if !ok {
		return nil
	}
	return state
}
