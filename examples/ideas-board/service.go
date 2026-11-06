package main

import (
	"context"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/pgxadapter"
	"github.com/jackc/pgx/v5"
)

// voteStore is the minimal read-modify-write surface the increment logic needs.
// The real implementation wraps a pgx.Tx; the unit test swaps in a fake so the
// pure logic can be tested without a database.
type voteStore interface {
	CurrentVotes(ctx context.Context, id string) (int, error)
	UpdateVotes(ctx context.Context, id string, votes int) error
}

// applyUpvote is the pure read-increment-save logic, independent of any
// transaction or database. This is what the unit test exercises.
func applyUpvote(ctx context.Context, store voteStore, id string) error {
	votes, err := store.CurrentVotes(ctx, id)
	if err != nil {
		return err
	}
	return store.UpdateVotes(ctx, id, votes+1)
}

// txStore adapts a pgx.Tx to voteStore so applyUpvote can run inside a transaction.
type txStore struct {
	ctx context.Context
	tx  pgx.Tx
}

func (s txStore) CurrentVotes(ctx context.Context, id string) (int, error) {
	return currentVotes(ctx, s.tx, id)
}

func (s txStore) UpdateVotes(ctx context.Context, id string, votes int) error {
	return updateVotes(ctx, s.tx, id, votes)
}

// Service upvotes ideas against a CockroachDB-aware pool.
type Service struct {
	pool *pgxadapter.Pool
}

// NewService builds a Service over the given pool.
func NewService(pool *pgxadapter.Pool) *Service {
	return &Service{pool: pool}
}

// tag::plain[]

// UpvoteWithoutRetry runs read-modify-write in a plain transaction with NO retry.
// Under concurrent contention CockroachDB aborts conflicting SERIALIZABLE
// transactions with SQLSTATE 40001, and this method returns that error — the
// vote is simply lost.
func (s *Service) UpvoteWithoutRetry(ctx context.Context, id string) error {
	tx, err := s.pool.RawPool().Begin(ctx)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL SERIALIZABLE"); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	votes, err := currentVotes(ctx, tx, id) // <1>
	if err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := updateVotes(ctx, tx, id, votes+1); err != nil { // <2>
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx) // <3>
}

// end::plain[]

// tag::retry[]

// Upvote runs the same read-modify-write, but inside ExecuteTx, which transparently
// retries the whole closure on SQLSTATE 40001 serialization conflicts. Every
// concurrent vote eventually lands.
func (s *Service) Upvote(ctx context.Context, id string) error {
	return s.pool.ExecuteTx(ctx, func(tx pgx.Tx) error { // <1>
		return applyUpvote(ctx, txStore{ctx: ctx, tx: tx}, id)
	})
}

// end::retry[]

// Votes returns the current vote count for an idea (non-transactional read).
func (s *Service) Votes(ctx context.Context, id string) (int, error) {
	return currentVotes(ctx, s.pool.RawPool(), id)
}
