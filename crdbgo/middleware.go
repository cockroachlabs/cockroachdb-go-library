package crdbgo

import "context"

// TxFunc is a function executed within a CockroachDB transaction. It receives
// a context carrying transaction-scoped metadata (region, isolation level, etc.).
type TxFunc func(ctx context.Context) error

// Middleware wraps a [TxFunc] to add cross-cutting behavior such as logging,
// metrics recording, distributed tracing, or region injection. Middlewares are
// composable via [Chain].
type Middleware func(next TxFunc) TxFunc

// Chain composes middlewares left-to-right: Chain(a, b, c)(fn) = a(b(c(fn))).
// The first middleware in the list is the outermost wrapper.
func Chain(middlewares ...Middleware) Middleware {
	return func(next TxFunc) TxFunc {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}
