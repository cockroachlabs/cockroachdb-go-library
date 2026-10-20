package crdbgo

import (
	"context"
	"errors"
)

// RegionContext holds multi-region routing metadata for CockroachDB's
// multi-region SQL features. Attach it to a context.Context via
// [WithRegionContext] so that adapters can route queries to the correct
// region or gateway.
type RegionContext struct {
	Region     string
	Gateway    string
	Localities map[string]string
}

type regionContextKey struct{}

// WithRegionContext attaches a [RegionContext] to the given context. Adapters
// extract it via [RegionFromContext] to apply region-aware query routing.
func WithRegionContext(ctx context.Context, rc *RegionContext) context.Context {
	return context.WithValue(ctx, regionContextKey{}, rc)
}

// RegionFromContext retrieves the [RegionContext] from ctx, if present.
// The second return value reports whether a RegionContext was found.
func RegionFromContext(ctx context.Context) (*RegionContext, bool) {
	rc, ok := ctx.Value(regionContextKey{}).(*RegionContext)
	return rc, ok
}

// RegionResolver resolves the current CockroachDB region from external sources
// such as environment variables, cloud instance metadata APIs, or DNS.
// Implement this interface to provide custom region detection logic.
type RegionResolver interface {
	Resolve(ctx context.Context) (string, error)
}

// StaticRegionResolver always returns a fixed, pre-configured region string.
// Useful for single-region deployments or testing.
type StaticRegionResolver struct {
	Region string
}

// Resolve returns the statically configured region.
func (r *StaticRegionResolver) Resolve(_ context.Context) (string, error) {
	return r.Region, nil
}

// ChainRegionResolver tries multiple [RegionResolver] implementations in order,
// returning the first successful non-empty result. This enables fallback chains
// such as "try cloud metadata, then env var, then static default."
type ChainRegionResolver struct {
	Resolvers []RegionResolver
}

// Resolve iterates through resolvers and returns the first non-empty result.
func (c *ChainRegionResolver) Resolve(ctx context.Context) (string, error) {
	for _, r := range c.Resolvers {
		region, err := r.Resolve(ctx)
		if err == nil && region != "" {
			return region, nil
		}
	}
	return "", errors.New("crdbgo: no region resolver succeeded")
}
