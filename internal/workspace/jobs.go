package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// JobKindPrepare is the durable job that runs workspace initialization
	// (clone, analysis, dependency setup, verification, and finalization).
	JobKindPrepare = "prepare_workspace"

	JobStateQueued    = "queued"
	JobStateRunning   = "running"
	JobStateSucceeded = "succeeded"
	JobStateFailed    = "failed"
	JobStateCanceled  = "canceled"

	jobLeaseOwner         = "controller"
	jobLeaseDuration      = 15 * time.Minute
	interruptedJobMessage = "Operation interrupted when Perpetual restarted"
)

var jobStates = map[string]bool{
	JobStateQueued: true, JobStateRunning: true, JobStateSucceeded: true,
	JobStateFailed: true, JobStateCanceled: true,
}

// Job is one durable workspace operation owned by the control plane. A worker
// claims queued jobs and records their outcome; browser requests only enqueue.
type Job struct {
	ID             string
	WorkspaceID    string
	Kind           string
	State          string
	Attempts       int
	MaxAttempts    int
	LeaseOwner     string
	LeaseExpiresAt time.Time
	Payload        string
	Error          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	StartedAt      time.Time
	FinishedAt     time.Time
}

const workspaceJobSchema = `CREATE TABLE IF NOT EXISTS workspace_jobs (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	kind TEXT NOT NULL,
	state TEXT NOT NULL,
	attempts INTEGER NOT NULL DEFAULT 0,
	max_attempts INTEGER NOT NULL DEFAULT 1,
	lease_owner TEXT NOT NULL DEFAULT '',
	lease_expires_at TEXT NOT NULL DEFAULT '',
	payload TEXT NOT NULL DEFAULT '',
	error TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	started_at TEXT NOT NULL DEFAULT '',
	finished_at TEXT NOT NULL DEFAULT ''
)`

func (s *Store) migrateWorkspaceJobs(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, workspaceJobSchema); err != nil {
		return fmt.Errorf("create workspace jobs: %w", err)
	}
	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS workspace_jobs_claim ON workspace_jobs(state, created_at)`,
		`CREATE INDEX IF NOT EXISTS workspace_jobs_workspace ON workspace_jobs(workspace_id, state)`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create workspace job index: %w", err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA user_version = 13`); err != nil {
		return fmt.Errorf("record workspace job schema: %w", err)
	}
	return nil
}

func (s *Store) CreateJob(ctx context.Context, workspaceID, kind string) (Job, error) {
	workspaceID, kind = strings.TrimSpace(workspaceID), strings.TrimSpace(kind)
	if workspaceID == "" || kind == "" {
		return Job{}, errors.New("workspace ID and job kind are required")
	}
	id, err := newID()
	if err != nil {
		return Job{}, err
	}
	now := time.Now().UTC()
	value := Job{
		ID: id, WorkspaceID: workspaceID, Kind: kind, State: JobStateQueued,
		MaxAttempts: 1, CreatedAt: now, UpdatedAt: now,
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO workspace_jobs (
		id, workspace_id, kind, state, attempts, max_attempts, payload, created_at, updated_at
	) VALUES (?, ?, ?, ?, 0, ?, '', ?, ?)`, value.ID, value.WorkspaceID, value.Kind,
		value.State, value.MaxAttempts, formatTime(value.CreatedAt), formatTime(value.UpdatedAt))
	if err != nil {
		return Job{}, fmt.Errorf("create workspace job: %w", err)
	}
	return value, nil
}

func (s *Store) HasActiveJob(ctx context.Context, workspaceID, kind string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM workspace_jobs
		WHERE workspace_id = ? AND kind = ? AND state IN (?, ?) LIMIT 1`,
		strings.TrimSpace(workspaceID), strings.TrimSpace(kind), JobStateQueued, JobStateRunning).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect workspace jobs: %w", err)
	}
	return true, nil
}

func (s *Store) Jobs(ctx context.Context, workspaceID string) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, kind, state, attempts,
		max_attempts, lease_owner, lease_expires_at, payload, error, created_at, updated_at,
		started_at, finished_at FROM workspace_jobs WHERE workspace_id = ? ORDER BY created_at`,
		strings.TrimSpace(workspaceID))
	if err != nil {
		return nil, fmt.Errorf("list workspace jobs: %w", err)
	}
	defer rows.Close()
	var values []Job
	for rows.Next() {
		value, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) ClaimNextJob(ctx context.Context) (Job, error) {
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, fmt.Errorf("begin job claim: %w", err)
	}
	defer tx.Rollback()
	var value Job
	if err := scanJobRow(tx.QueryRowContext(ctx, `SELECT id, workspace_id, kind, state, attempts,
		max_attempts, lease_owner, lease_expires_at, payload, error, created_at, updated_at,
		started_at, finished_at FROM workspace_jobs WHERE state = ? ORDER BY created_at LIMIT 1`,
		JobStateQueued), &value); err != nil {
		return Job{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE workspace_jobs SET state = ?, attempts = attempts + 1,
		lease_owner = ?, lease_expires_at = ?, started_at = ?, updated_at = ?
		WHERE id = ? AND state = ?`, JobStateRunning, jobLeaseOwner,
		formatTime(now.Add(jobLeaseDuration)), formatTime(now), formatTime(now),
		value.ID, JobStateQueued)
	if err != nil {
		return Job{}, fmt.Errorf("claim workspace job: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return Job{}, err
	}
	if err := tx.Commit(); err != nil {
		return Job{}, fmt.Errorf("commit job claim: %w", err)
	}
	value.State = JobStateRunning
	value.Attempts++
	value.LeaseOwner = jobLeaseOwner
	value.LeaseExpiresAt = now.Add(jobLeaseDuration)
	value.StartedAt = now
	value.UpdatedAt = now
	return value, nil
}

func (s *Store) FinishJob(ctx context.Context, id, state, message string) error {
	if !jobStates[state] {
		return fmt.Errorf("invalid job state %q", state)
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE workspace_jobs SET state = ?, error = ?,
		finished_at = ?, updated_at = ? WHERE id = ? AND state = ?`,
		state, strings.TrimSpace(message), formatTime(now), formatTime(now),
		strings.TrimSpace(id), JobStateRunning)
	if err != nil {
		return fmt.Errorf("finish workspace job: %w", err)
	}
	return requireOneRow(result)
}

func (s *Store) RecoverJobs(ctx context.Context) error {
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, `UPDATE workspace_jobs SET state = ?, error = ?,
		finished_at = ?, updated_at = ? WHERE state IN (?, ?)`,
		JobStateFailed, interruptedJobMessage, now, now, JobStateQueued, JobStateRunning)
	if err != nil {
		return fmt.Errorf("recover workspace jobs: %w", err)
	}
	return nil
}

func scanJob(row scanner) (Job, error) {
	var value Job
	if err := scanJobRow(row, &value); err != nil {
		return Job{}, err
	}
	return value, nil
}

func scanJobRow(row scanner, value *Job) error {
	var leaseExpiresAt, createdAt, updatedAt, startedAt, finishedAt string
	err := row.Scan(&value.ID, &value.WorkspaceID, &value.Kind, &value.State,
		&value.Attempts, &value.MaxAttempts, &value.LeaseOwner, &leaseExpiresAt,
		&value.Payload, &value.Error, &createdAt, &updatedAt, &startedAt, &finishedAt)
	if err != nil {
		return err
	}
	for _, pair := range []struct {
		encoded string
		target  *time.Time
	}{
		{leaseExpiresAt, &value.LeaseExpiresAt},
		{createdAt, &value.CreatedAt},
		{updatedAt, &value.UpdatedAt},
		{startedAt, &value.StartedAt},
		{finishedAt, &value.FinishedAt},
	} {
		if pair.encoded == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, pair.encoded)
		if err != nil {
			return fmt.Errorf("decode workspace job time: %w", err)
		}
		*pair.target = parsed
	}
	return nil
}
