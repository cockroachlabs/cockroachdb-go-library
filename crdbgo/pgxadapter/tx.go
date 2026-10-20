package pgxadapter

import (
	"context"
	"fmt"
	"time"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo/continuum/vcluster"
	"github.com/jackc/pgx/v5"
)

// ExecuteTxOptions overrides the pool-level transaction settings for a single
// [Pool.ExecuteTxWithOptions] call. Nil fields fall back to the pool's [crdbgo.Config].
type ExecuteTxOptions struct {
	ReadOnly       *bool
	IsolationLevel *string
}

// ExecuteTx runs fn within a CockroachDB transaction, automatically retrying on
// serialization failures (SQLSTATE 40001 / CR000) according to the pool's
// [crdbgo.RetryPolicy]. It uses the pool-level isolation and read-only settings.
func (p *Pool) ExecuteTx(ctx context.Context, fn func(pgx.Tx) error) error {
	return p.ExecuteTxWithOptions(ctx, fn, nil)
}

// ExecuteTxWithOptions runs fn within a CockroachDB transaction with per-call
// overrides for isolation level and read-only mode. Pass nil for txOpts to use
// the pool defaults (equivalent to [Pool.ExecuteTx]).
func (p *Pool) ExecuteTxWithOptions(ctx context.Context, fn func(pgx.Tx) error, txOpts *ExecuteTxOptions) error {
	start := time.Now()
	retries := 0

	isoLevel := p.config.IsolationLevel.String()
	readOnly := p.config.ReadOnly
	if txOpts != nil {
		if txOpts.IsolationLevel != nil {
			isoLevel = *txOpts.IsolationLevel
		}
		if txOpts.ReadOnly != nil {
			readOnly = *txOpts.ReadOnly
		}
	}

	for {
		err := p.executeSingleTx(ctx, fn, isoLevel, readOnly)
		if err == nil {
			if p.metrics != nil {
				p.metrics.RecordTransaction(time.Since(start), retries, true)
			}
			return nil
		}

		if !IsRetryable(err) {
			if p.metrics != nil {
				p.metrics.RecordTransaction(time.Since(start), retries, false)
			}
			return err
		}

		if !p.config.Retry.ShouldRetry(retries) {
			if p.metrics != nil {
				p.metrics.RecordTransaction(time.Since(start), retries, false)
			}
			return err
		}

		delay := p.config.Retry.NextDelay(retries)
		if p.metrics != nil {
			p.metrics.RecordRetry(delay)
		}
		retries++

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

func (p *Pool) executeSingleTx(ctx context.Context, fn func(pgx.Tx) error, isoLevel string, readOnly bool) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, fmt.Sprintf("SET TRANSACTION ISOLATION LEVEL %s", isoLevel)); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}

	if readOnly {
		if _, err := tx.Exec(ctx, "SET TRANSACTION READ ONLY"); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
	}

	// Cockroach vCluster: when the context carries a virtual-cluster / workload
	// tag, apply it to the session for the life of this transaction. set_config
	// with is_local=true has SET LOCAL semantics (scoped to the transaction) and,
	// unlike a literal SET statement, accepts a placeholder so the customer-
	// supplied vCluster name is bound safely rather than string-interpolated.
	if tag, ok := vcluster.FromContext(ctx); ok {
		if tag.VirtualCluster != "" {
			if _, err := tx.Exec(ctx,
				"SELECT set_config('application_name', $1, true)", tag.VirtualCluster); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
		}
		if tag.QoS != "" {
			if _, err := tx.Exec(ctx,
				"SELECT set_config('default_transaction_quality_of_service', $1, true)", tag.QoS); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
		}
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}

	return tx.Commit(ctx)
}
