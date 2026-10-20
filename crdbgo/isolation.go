package crdbgo

// IsolationLevel represents a CockroachDB transaction isolation level.
// Use [Serializable] (default) for the strongest consistency guarantee, or
// [ReadCommitted] to reduce retry frequency at the cost of weaker isolation.
type IsolationLevel int

const (
	// Serializable is the default CockroachDB isolation level providing
	// linearizable consistency. Transactions may be automatically retried
	// on serialization conflicts (SQLSTATE 40001).
	Serializable IsolationLevel = iota

	// ReadCommitted allows transactions to see committed data from other
	// transactions, reducing retries at the cost of weaker consistency.
	// Available in CockroachDB v23.1+.
	ReadCommitted
)

// String returns the SQL representation of the isolation level
// (e.g., "SERIALIZABLE" or "READ COMMITTED").
func (l IsolationLevel) String() string {
	switch l {
	case ReadCommitted:
		return "READ COMMITTED"
	default:
		return "SERIALIZABLE"
	}
}
