package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// tag::repo[]

// querier is the subset of pgx shared by *pgxpool.Pool and pgx.Tx, so the same
// read-modify-write helpers work on a pool (setup/seed) or inside a transaction
// (the upvote path).
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

const createTableSQL = `CREATE TABLE IF NOT EXISTS ideas (
	id    STRING PRIMARY KEY,
	text  STRING NOT NULL,
	votes INT    NOT NULL DEFAULT 0
)`

// ensureSchema creates the ideas table if it does not already exist.
func ensureSchema(ctx context.Context, q querier) error {
	_, err := q.Exec(ctx, createTableSQL)
	return err
}

// seedIdea inserts the idea, leaving an existing row untouched.
func seedIdea(ctx context.Context, q querier, idea Idea) error {
	_, err := q.Exec(ctx,
		"INSERT INTO ideas (id, text, votes) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING",
		idea.ID, idea.Text, idea.Votes)
	return err
}

// find returns the full idea row.
func find(ctx context.Context, q querier, id string) (Idea, error) {
	var idea Idea
	err := q.QueryRow(ctx, "SELECT id, text, votes FROM ideas WHERE id = $1", id).
		Scan(&idea.ID, &idea.Text, &idea.Votes)
	return idea, err
}

// currentVotes reads just the vote count — the "read" half of read-modify-write.
func currentVotes(ctx context.Context, q querier, id string) (int, error) {
	var votes int
	err := q.QueryRow(ctx, "SELECT votes FROM ideas WHERE id = $1", id).Scan(&votes)
	return votes, err
}

// updateVotes writes an absolute vote count — the "save" half of read-modify-write.
func updateVotes(ctx context.Context, q querier, id string, votes int) error {
	_, err := q.Exec(ctx, "UPDATE ideas SET votes = $1 WHERE id = $2", votes, id)
	return err
}

// reset sets the vote count back to zero so the two demo runs start even.
func reset(ctx context.Context, q querier, id string) error {
	if _, err := q.Exec(ctx, "UPDATE ideas SET votes = 0 WHERE id = $1", id); err != nil {
		return fmt.Errorf("reset votes: %w", err)
	}
	return nil
}

// end::repo[]
