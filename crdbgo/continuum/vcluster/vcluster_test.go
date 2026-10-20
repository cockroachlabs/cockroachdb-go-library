package vcluster

import (
	"context"
	"testing"
)

func TestPriorityQoS(t *testing.T) {
	cases := []struct {
		p    Priority
		want string
	}{
		{High, "critical"},
		{Normal, "regular"},
		{Low, "background"},
		{Priority(99), "regular"}, // unknown → default
	}
	for _, c := range cases {
		if got := c.p.QoS(); got != c.want {
			t.Errorf("Priority(%d).QoS() = %q, want %q", c.p, got, c.want)
		}
	}
}

func TestPriorityString(t *testing.T) {
	if High.String() != "High" || Normal.String() != "Normal" || Low.String() != "Low" {
		t.Errorf("unexpected Priority.String(): High=%q Normal=%q Low=%q",
			High.String(), Normal.String(), Low.String())
	}
}

func TestFromContext_Empty(t *testing.T) {
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("expected no vCluster tag on a plain context")
	}
}

func TestWith_NameAndPriority(t *testing.T) {
	ctx := With(context.Background(), "analytics", Low)
	tag, ok := FromContext(ctx)
	if !ok {
		t.Fatal("expected a tag")
	}
	if tag.VirtualCluster != "analytics" {
		t.Errorf("VirtualCluster = %q, want %q", tag.VirtualCluster, "analytics")
	}
	if tag.QoS != "background" {
		t.Errorf("QoS = %q, want %q", tag.QoS, "background")
	}
}

func TestWithContext_OptionsCompose(t *testing.T) {
	// Set the cluster in one call, the workload in a later one; both must survive.
	ctx := WithContext(context.Background(), WithVirtualCluster("oltp"))
	ctx = WithContext(ctx, WithWorkload(High))
	tag, ok := FromContext(ctx)
	if !ok {
		t.Fatal("expected a tag")
	}
	if tag.VirtualCluster != "oltp" {
		t.Errorf("VirtualCluster = %q, want %q", tag.VirtualCluster, "oltp")
	}
	if tag.QoS != "critical" {
		t.Errorf("QoS = %q, want %q", tag.QoS, "critical")
	}
}

func TestWithContext_NameOnly_NoQoS(t *testing.T) {
	ctx := WithContext(context.Background(), WithVirtualCluster("reporting"))
	tag, ok := FromContext(ctx)
	if !ok {
		t.Fatal("expected a tag")
	}
	if tag.VirtualCluster != "reporting" {
		t.Errorf("VirtualCluster = %q, want %q", tag.VirtualCluster, "reporting")
	}
	if tag.QoS != "" {
		t.Errorf("QoS = %q, want empty (no workload set)", tag.QoS)
	}
}
