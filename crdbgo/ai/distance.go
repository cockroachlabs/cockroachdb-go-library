package ai

import "fmt"

// DistanceMetric defines the vector distance function used for similarity search.
// CockroachDB supports three native distance operators.
type DistanceMetric int

const (
	// L2 uses Euclidean distance (operator: <->).
	L2 DistanceMetric = iota
	// Cosine uses cosine distance (operator: <=>). Best for text embeddings.
	Cosine
	// InnerProduct uses negative inner product (operator: <#>).
	InnerProduct
)

var distanceNames = [...]string{"L2", "Cosine", "InnerProduct"}
var distanceOperators = [...]string{"<->", "<=>", "<#>"}
var distanceOpClasses = [...]string{"vector_l2_ops", "vector_cosine_ops", "vector_ip_ops"}

// String returns the human-readable metric name ("L2", "Cosine", "InnerProduct").
func (d DistanceMetric) String() string {
	if d < 0 || int(d) >= len(distanceNames) {
		return fmt.Sprintf("DistanceMetric(%d)", d)
	}
	return distanceNames[d]
}

// Operator returns the CockroachDB SQL operator for this distance metric.
func (d DistanceMetric) Operator() string {
	if d < 0 || int(d) >= len(distanceOperators) {
		return "<->"
	}
	return distanceOperators[d]
}

// OpClass returns the CockroachDB vector index operator class for this distance
// metric (e.g., "vector_cosine_ops", "vector_l2_ops", "vector_ip_ops").
// Used with CREATE VECTOR INDEX in CockroachDB v26.2+.
func (d DistanceMetric) OpClass() string {
	if d < 0 || int(d) >= len(distanceOpClasses) {
		return "vector_l2_ops"
	}
	return distanceOpClasses[d]
}

// ToSimilarity converts a raw distance value to a [0,1] similarity score.
// Higher similarity means more similar.
func (d DistanceMetric) ToSimilarity(distance float64) float64 {
	switch d {
	case Cosine:
		return 1.0 - distance
	case InnerProduct:
		return -distance
	case L2:
		return 1.0 / (1.0 + distance)
	default:
		return 1.0 / (1.0 + distance)
	}
}
