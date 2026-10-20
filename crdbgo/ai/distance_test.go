package ai

import (
	"math"
	"testing"
)

func TestDistanceMetric_String(t *testing.T) {
	tests := []struct {
		metric DistanceMetric
		want   string
	}{
		{L2, "L2"},
		{Cosine, "Cosine"},
		{InnerProduct, "InnerProduct"},
		{DistanceMetric(99), "DistanceMetric(99)"},
	}
	for _, tt := range tests {
		if got := tt.metric.String(); got != tt.want {
			t.Errorf("%d.String() = %q, want %q", tt.metric, got, tt.want)
		}
	}
}

func TestDistanceMetric_Operator(t *testing.T) {
	tests := []struct {
		metric DistanceMetric
		want   string
	}{
		{L2, "<->"},
		{Cosine, "<=>"},
		{InnerProduct, "<#>"},
		{DistanceMetric(-1), "<->"},
	}
	for _, tt := range tests {
		if got := tt.metric.Operator(); got != tt.want {
			t.Errorf("%s.Operator() = %q, want %q", tt.metric, got, tt.want)
		}
	}
}

func TestDistanceMetric_OpClass(t *testing.T) {
	tests := []struct {
		metric DistanceMetric
		want   string
	}{
		{L2, "vector_l2_ops"},
		{Cosine, "vector_cosine_ops"},
		{InnerProduct, "vector_ip_ops"},
		{DistanceMetric(-1), "vector_l2_ops"},
	}
	for _, tt := range tests {
		if got := tt.metric.OpClass(); got != tt.want {
			t.Errorf("%s.OpClass() = %q, want %q", tt.metric, got, tt.want)
		}
	}
}

func TestDistanceMetric_ToSimilarity(t *testing.T) {
	tests := []struct {
		metric   DistanceMetric
		distance float64
		want     float64
	}{
		{Cosine, 0.0, 1.0},
		{Cosine, 0.3, 0.7},
		{Cosine, 1.0, 0.0},
		{InnerProduct, -0.9, 0.9},
		{InnerProduct, 0.0, 0.0},
		{L2, 0.0, 1.0},
		{L2, 1.0, 0.5},
	}
	for _, tt := range tests {
		got := tt.metric.ToSimilarity(tt.distance)
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("%s.ToSimilarity(%f) = %f, want %f", tt.metric, tt.distance, got, tt.want)
		}
	}
}
