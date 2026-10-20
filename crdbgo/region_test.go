package crdbgo

import (
	"context"
	"errors"
	"testing"
)

func TestRegionContext_Roundtrip(t *testing.T) {
	ctx := context.Background()
	rc := &RegionContext{
		Region:  "us-east-1",
		Gateway: "gw-east",
		Localities: map[string]string{
			"zone": "a",
		},
	}

	ctx = WithRegionContext(ctx, rc)
	got, ok := RegionFromContext(ctx)
	if !ok {
		t.Fatal("RegionFromContext returned ok=false, want true")
	}
	if got.Region != "us-east-1" {
		t.Errorf("Region = %q, want us-east-1", got.Region)
	}
	if got.Gateway != "gw-east" {
		t.Errorf("Gateway = %q, want gw-east", got.Gateway)
	}
	if got.Localities["zone"] != "a" {
		t.Errorf("Localities[zone] = %q, want a", got.Localities["zone"])
	}
}

func TestRegionFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	_, ok := RegionFromContext(ctx)
	if ok {
		t.Error("RegionFromContext on empty context returned ok=true, want false")
	}
}

func TestStaticRegionResolver(t *testing.T) {
	r := &StaticRegionResolver{Region: "eu-west-1"}
	region, err := r.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if region != "eu-west-1" {
		t.Errorf("region = %q, want eu-west-1", region)
	}
}

// failingResolver always returns an error.
type failingResolver struct {
	err error
}

func (f *failingResolver) Resolve(_ context.Context) (string, error) {
	return "", f.err
}

func TestChainRegionResolver_FirstSuccess(t *testing.T) {
	chain := &ChainRegionResolver{
		Resolvers: []RegionResolver{
			&failingResolver{err: errors.New("cloud metadata unavailable")},
			&StaticRegionResolver{Region: "us-west-2"},
			&StaticRegionResolver{Region: "eu-central-1"},
		},
	}

	region, err := chain.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if region != "us-west-2" {
		t.Errorf("region = %q, want us-west-2 (first successful)", region)
	}
}

func TestChainRegionResolver_AllFail(t *testing.T) {
	chain := &ChainRegionResolver{
		Resolvers: []RegionResolver{
			&failingResolver{err: errors.New("fail 1")},
			&failingResolver{err: errors.New("fail 2")},
		},
	}

	_, err := chain.Resolve(context.Background())
	if err == nil {
		t.Fatal("Resolve returned nil error, want error when all resolvers fail")
	}
}
