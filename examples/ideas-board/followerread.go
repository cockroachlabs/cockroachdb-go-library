package main

import (
	"context"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/pgxadapter"
)

// tag::follower-read[]

// IdeaReadService serves leaderboard reads from a follower replica. Each query
// runs AS OF SYSTEM TIME follower_read_timestamp() (a slightly-stale snapshot the
// nearest replica can serve), so a hot read path avoids the leaseholder
// round-trip. Writes still go through Service; this type is read-only.
type IdeaReadService struct {
	pool *pgxadapter.Pool
}

// NewIdeaReadService builds a follower-read view over the given pool.
func NewIdeaReadService(pool *pgxadapter.Pool) *IdeaReadService {
	return &IdeaReadService{pool: pool}
}

// TopIdeas returns the highest-voted ideas, read from a follower. Because the
// read is historical (follower_read_timestamp() is a few seconds stale), rows
// inserted within the staleness window are not yet visible — that trade is the
// whole point of a follower read.
func (s *IdeaReadService) TopIdeas(ctx context.Context, limit int) ([]Idea, error) {
	rows, err := s.pool.QueryFollowerRead(ctx,
		"SELECT id, text, votes FROM ideas ORDER BY votes DESC, text ASC LIMIT $1", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Idea
	for rows.Next() {
		var idea Idea
		if err := rows.Scan(&idea.ID, &idea.Text, &idea.Votes); err != nil {
			return nil, err
		}
		out = append(out, idea)
	}
	return out, rows.Err()
}

// end::follower-read[]
