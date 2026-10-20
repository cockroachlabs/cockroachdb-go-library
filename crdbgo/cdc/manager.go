package cdc

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ChangefeedInfo describes a running or completed CockroachDB changefeed job,
// as reported by SHOW CHANGEFEED JOB.
type ChangefeedInfo struct {
	JobID       int64
	Status      string
	Description string
	SinkURI     string
	Created     time.Time
	Modified    time.Time
}

// Querier is the minimal database interface needed by ChangefeedManager.
// Both *sql.DB and *sql.Tx satisfy this interface.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// ChangefeedManager creates and manages CockroachDB changefeed jobs using
// PAUSE/RESUME/CANCEL JOB SQL statements.
type ChangefeedManager struct {
	db Querier
}

// NewChangefeedManager creates a manager using the given database connection.
func NewChangefeedManager(db Querier) *ChangefeedManager {
	return &ChangefeedManager{db: db}
}

// Create executes the changefeed SQL and returns the job info.
func (m *ChangefeedManager) Create(ctx context.Context, spec *ChangefeedSpec) (*ChangefeedInfo, error) {
	query := spec.ToSQL()
	row := m.db.QueryRowContext(ctx, query)

	var jobID int64
	if err := row.Scan(&jobID); err != nil {
		return nil, fmt.Errorf("cdc: failed to create changefeed: %w", err)
	}

	return &ChangefeedInfo{
		JobID:   jobID,
		Status:  "running",
		Created: time.Now(),
	}, nil
}

// Pause pauses a running changefeed job.
func (m *ChangefeedManager) Pause(ctx context.Context, jobID int64) error {
	_, err := m.db.ExecContext(ctx, "PAUSE JOB $1", jobID)
	if err != nil {
		return fmt.Errorf("cdc: failed to pause job %d: %w", jobID, err)
	}
	return nil
}

// Resume resumes a paused changefeed job.
func (m *ChangefeedManager) Resume(ctx context.Context, jobID int64) error {
	_, err := m.db.ExecContext(ctx, "RESUME JOB $1", jobID)
	if err != nil {
		return fmt.Errorf("cdc: failed to resume job %d: %w", jobID, err)
	}
	return nil
}

// Cancel permanently cancels a changefeed job.
func (m *ChangefeedManager) Cancel(ctx context.Context, jobID int64) error {
	_, err := m.db.ExecContext(ctx, "CANCEL JOB $1", jobID)
	if err != nil {
		return fmt.Errorf("cdc: failed to cancel job %d: %w", jobID, err)
	}
	return nil
}

// Status retrieves the current status of a changefeed job.
func (m *ChangefeedManager) Status(ctx context.Context, jobID int64) (*ChangefeedInfo, error) {
	row := m.db.QueryRowContext(ctx,
		"SELECT job_id, status, description, COALESCE(sink_uri, ''), created, modified FROM [SHOW CHANGEFEED JOB $1]",
		jobID,
	)

	info := &ChangefeedInfo{}
	if err := row.Scan(&info.JobID, &info.Status, &info.Description, &info.SinkURI, &info.Created, &info.Modified); err != nil {
		return nil, fmt.Errorf("cdc: failed to get job status %d: %w", jobID, err)
	}
	return info, nil
}

// List returns all changefeed jobs.
func (m *ChangefeedManager) List(ctx context.Context) ([]ChangefeedInfo, error) {
	rows, err := m.db.QueryContext(ctx,
		"SELECT job_id, status, description, COALESCE(sink_uri, ''), created, modified FROM [SHOW CHANGEFEED JOBS]",
	)
	if err != nil {
		return nil, fmt.Errorf("cdc: failed to list changefeed jobs: %w", err)
	}
	defer rows.Close()

	var jobs []ChangefeedInfo
	for rows.Next() {
		var info ChangefeedInfo
		if err := rows.Scan(&info.JobID, &info.Status, &info.Description, &info.SinkURI, &info.Created, &info.Modified); err != nil {
			return nil, fmt.Errorf("cdc: failed to scan changefeed job: %w", err)
		}
		jobs = append(jobs, info)
	}
	return jobs, rows.Err()
}
