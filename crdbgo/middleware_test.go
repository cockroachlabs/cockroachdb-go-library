package crdbgo

import (
	"context"
	"testing"
)

func TestChain_SingleMiddleware(t *testing.T) {
	called := false
	mw := func(next TxFunc) TxFunc {
		return func(ctx context.Context) error {
			called = true
			return next(ctx)
		}
	}

	fn := Chain(mw)(func(ctx context.Context) error {
		return nil
	})

	if err := fn(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("middleware was not called")
	}
}

func TestChain_MultipleMiddlewares_Order(t *testing.T) {
	var order []string

	mwA := func(next TxFunc) TxFunc {
		return func(ctx context.Context) error {
			order = append(order, "A-before")
			err := next(ctx)
			order = append(order, "A-after")
			return err
		}
	}

	mwB := func(next TxFunc) TxFunc {
		return func(ctx context.Context) error {
			order = append(order, "B-before")
			err := next(ctx)
			order = append(order, "B-after")
			return err
		}
	}

	mwC := func(next TxFunc) TxFunc {
		return func(ctx context.Context) error {
			order = append(order, "C-before")
			err := next(ctx)
			order = append(order, "C-after")
			return err
		}
	}

	fn := Chain(mwA, mwB, mwC)(func(ctx context.Context) error {
		order = append(order, "handler")
		return nil
	})

	if err := fn(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"A-before", "B-before", "C-before", "handler", "C-after", "B-after", "A-after"}
	if len(order) != len(expected) {
		t.Fatalf("execution order length = %d, want %d: %v", len(order), len(expected), order)
	}
	for i, want := range expected {
		if order[i] != want {
			t.Errorf("order[%d] = %q, want %q", i, order[i], want)
		}
	}
}

func TestChain_NoMiddlewares(t *testing.T) {
	handlerCalled := false
	fn := Chain()(func(ctx context.Context) error {
		handlerCalled = true
		return nil
	})

	if err := fn(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handlerCalled {
		t.Error("handler was not called")
	}
}

type testContextKey struct{}

func TestChain_MiddlewareAddsContext(t *testing.T) {
	mw := func(next TxFunc) TxFunc {
		return func(ctx context.Context) error {
			ctx = context.WithValue(ctx, testContextKey{}, "injected")
			return next(ctx)
		}
	}

	var gotValue string
	fn := Chain(mw)(func(ctx context.Context) error {
		v, _ := ctx.Value(testContextKey{}).(string)
		gotValue = v
		return nil
	})

	if err := fn(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotValue != "injected" {
		t.Errorf("context value = %q, want injected", gotValue)
	}
}
