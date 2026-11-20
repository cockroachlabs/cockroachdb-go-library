package gormadapter

import (
	"context"
	"time"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
	"gorm.io/gorm"
)

// retryContextKey is used to store retry state across before/after callbacks.
type retryContextKey struct{}

// regionSettingKey is the GORM instance-setting key that [InRegion] writes and
// homeRegionCallback reads to home a created row's crdb_region column.
const regionSettingKey = "crdbgo:region"

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

	db.Statement.Context = context.WithValue(ctx, retryContextKey{}, state)
}

// resolveRegion returns the region to home a created row in, preferring (in
// order) the per-call [InRegion] setting, a RegionContext on the statement
// context, then the plugin-level configured region. Returns "" when none apply.
func (p *Plugin) resolveRegion(db *gorm.DB) string {
	if v, ok := db.Get(regionSettingKey); ok {
		if region, ok := v.(string); ok && region != "" {
			return region
		}
	}
	if db.Statement != nil {
		if rc, ok := crdbgo.RegionFromContext(db.Statement.Context); ok && rc.Region != "" {
			return rc.Region
		}
	}
	return p.config.Region
}

// homeRegionCallback runs before gorm:create and homes the new row in the
// resolved region by stamping the hidden crdb_region column. CockroachDB has no
// crdb_region session variable, so a REGIONAL BY ROW row is homed by writing the
// column, not with SET. It is a no-op when no region is resolved or the model
// does not map a crdb_region column; it is registered on Create only, so updates
// never silently relocate existing rows.
func (p *Plugin) homeRegionCallback(db *gorm.DB) {
	if db.Statement == nil || db.Statement.Schema == nil {
		return
	}
	region := p.resolveRegion(db)
	if region == "" {
		return
	}
	if field := db.Statement.Schema.LookUpField("crdb_region"); field == nil {
		return
	}
	db.Statement.SetColumn("crdb_region", region)
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
