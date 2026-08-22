package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	appdatabase "github.com/Saieshwar5/perpetual/internal/database"
)

// ErrNoRuns is returned when no queued execution room is available.
var ErrNoRuns = errors.New("no queued execution rooms")

// ClaimNextRun claims one queued execution room for a worker. Postgres uses
// FOR UPDATE SKIP LOCKED so concurrent workers never claim the same run.
func (s *Store) ClaimNextRun(ctx context.Context) (Run, error) {
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, fmt.Errorf("begin run claim: %w", err)
	}
	defer tx.Rollback()
	claimQuery := `SELECT id, user_id, workspace_id, session_id, state,
		step_cursor, max_steps, deadline_at, lease_owner, lease_expires_at,
		heartbeat_at, current_command, result, error, created_at, updated_at,
		started_at, finished_at FROM agent_runs WHERE state = ? ORDER BY created_at LIMIT 1`
	if s.dialect == appdatabase.ProviderPostgres {
		claimQuery += ` FOR UPDATE SKIP LOCKED`
	}
	value, err := scanRunRow(s.queryRowTx(ctx, tx, claimQuery, RunQueued))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Run{}, ErrNoRuns
		}
		return Run{}, err
	}
	nowText := formatTime(now)
	leaseText := formatTime(now.Add(runLeaseDuration))
	result, err := s.execTx(ctx, tx, `UPDATE agent_runs SET state = ?, lease_owner = ?,
		lease_expires_at = ?, heartbeat_at = ?, started_at = ?, updated_at = ?
		WHERE id = ? AND state = ?`,
		RunRunning, runLeaseOwner, leaseText, nowText, nowText, nowText,
		value.ID, RunQueued)
	if err != nil {
		return Run{}, fmt.Errorf("claim execution room: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return Run{}, err
	}
	if err := tx.Commit(); err != nil {
		return Run{}, fmt.Errorf("commit run claim: %w", err)
	}
	value.State = RunRunning
	value.LeaseOwner = runLeaseOwner
	value.LeaseExpiresAt = now.Add(runLeaseDuration)
	value.HeartbeatAt = now
	value.StartedAt = now
	return value, nil
}

// TouchRunLease extends the lease and records a heartbeat. Workers call this
// periodically while actively processing a run.
func (s *Store) TouchRunLease(ctx context.Context, runID string) error {
	now := time.Now().UTC()
	result, err := s.execContext(ctx, `UPDATE agent_runs SET lease_expires_at = ?,
		heartbeat_at = ?, updated_at = ? WHERE id = ? AND state = ?`,
		formatTime(now.Add(runLeaseDuration)), formatTime(now), formatTime(now),
		strings.TrimSpace(runID), RunRunning)
	if err != nil {
		return fmt.Errorf("touch execution room lease: %w", err)
	}
	return requireOneRow(result)
}

// CompleteRun marks an execution room completed with an optional result.
func (s *Store) CompleteRun(ctx context.Context, runID, result string) error {
	now := formatTime(time.Now().UTC())
	_, err := s.execContext(ctx, `UPDATE agent_runs SET state = ?, result = ?,
		finished_at = ?, updated_at = ?, lease_owner = '', lease_expires_at = ''
		WHERE id = ? AND state = ?`,
		RunCompleted, strings.TrimSpace(result), now, now,
		strings.TrimSpace(runID), RunRunning)
	if err != nil {
		return fmt.Errorf("complete execution room: %w", err)
	}
	return nil
}

// FailRun records a durable failure for an execution room.
func (s *Store) FailRun(ctx context.Context, runID, message string) error {
	now := formatTime(time.Now().UTC())
	_, err := s.execContext(ctx, `UPDATE agent_runs SET state = ?, error = ?,
		finished_at = ?, updated_at = ?, lease_owner = '', lease_expires_at = ''
		WHERE id = ?`,
		RunFailed, strings.TrimSpace(message), now, now, strings.TrimSpace(runID))
	if err != nil {
		return fmt.Errorf("fail execution room: %w", err)
	}
	return nil
}

// CancelRun marks an execution room canceled without a terminal error.
func (s *Store) CancelRun(ctx context.Context, runID string) error {
	now := formatTime(time.Now().UTC())
	_, err := s.execContext(ctx, `UPDATE agent_runs SET state = ?, finished_at = ?,
		updated_at = ?, lease_owner = '', lease_expires_at = ''
		WHERE id = ?`,
		RunCanceled, now, now, strings.TrimSpace(runID))
	if err != nil {
		return fmt.Errorf("cancel execution room: %w", err)
	}
	return nil
}

// SetRunWaitingUser pauses the loop until the user replies.
func (s *Store) SetRunWaitingUser(ctx context.Context, runID string) error {
	now := formatTime(time.Now().UTC())
	_, err := s.execContext(ctx, `UPDATE agent_runs SET state = ?, updated_at = ?
		WHERE id = ?`, RunWaitingUser, now, strings.TrimSpace(runID))
	if err != nil {
		return fmt.Errorf("pause execution room: %w", err)
	}
	return nil
}

// RunSteps returns the durable step journal for one run, oldest first.
func (s *Store) RunSteps(ctx context.Context, runID string) ([]RunStep, error) {
	rows, err := s.queryContext(ctx, `SELECT run_id, step_key, kind, status,
		input, output, created_at, done_at FROM agent_run_steps
		WHERE run_id = ? ORDER BY created_at`, strings.TrimSpace(runID))
	if err != nil {
		return nil, fmt.Errorf("list execution room steps: %w", err)
	}
	defer rows.Close()
	var steps []RunStep
	for rows.Next() {
		step, err := scanRunStep(rows)
		if err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

func scanRunStep(row scanner) (RunStep, error) {
	var value RunStep
	var inputText, outputText, createdAt, doneAt string
	var status string
	if err := row.Scan(&value.RunID, &value.StepKey, &value.Kind, &status,
		&inputText, &outputText, &createdAt, &doneAt); err != nil {
		return RunStep{}, err
	}
	value.Status = status
	if err := json.Unmarshal([]byte(inputText), &value.Input); err != nil {
		return RunStep{}, fmt.Errorf("decode run step input: %w", err)
	}
	if err := json.Unmarshal([]byte(outputText), &value.Output); err != nil {
		return RunStep{}, fmt.Errorf("decode run step output: %w", err)
	}
	parsed, err := parseStoredTime(createdAt)
	if err != nil {
		return RunStep{}, err
	}
	value.CreatedAt = parsed
	if strings.TrimSpace(doneAt) != "" {
		parsed, err = parseStoredTime(doneAt)
		if err != nil {
			return RunStep{}, err
		}
		value.DoneAt = parsed
	}
	return value, nil
}

// SaveWorkMemory writes the run scratchpad with optimistic concurrency.
func (s *Store) SaveWorkMemory(ctx context.Context, runID string, notes map[string]any, version int64) error {
	encoded, err := json.Marshal(nonNilMap(notes))
	if err != nil {
		return fmt.Errorf("encode work memory: %w", err)
	}
	now := formatTime(time.Now().UTC())
	result, err := s.execContext(ctx, `INSERT INTO agent_work_memory (run_id, notes, version, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET notes = excluded.notes,
		version = excluded.version, updated_at = excluded.updated_at
		WHERE agent_work_memory.version = ?`,
		strings.TrimSpace(runID), string(encoded), version+1, now,
		version)
	if err != nil {
		return fmt.Errorf("save work memory: %w", err)
	}
	return requireOneRow(result)
}

// WorkMemory reads the run scratchpad.
func (s *Store) WorkMemory(ctx context.Context, runID string) (WorkMemory, error) {
	var value WorkMemory
	var notesText, updatedAt string
	row := s.queryRowContext(ctx, `SELECT run_id, notes, version, updated_at
		FROM agent_work_memory WHERE run_id = ?`, strings.TrimSpace(runID))
	if err := row.Scan(&value.RunID, &notesText, &value.Version, &updatedAt); err != nil {
		return WorkMemory{}, err
	}
	if err := json.Unmarshal([]byte(notesText), &value.Notes); err != nil {
		return WorkMemory{}, fmt.Errorf("decode work memory: %w", err)
	}
	parsed, err := parseStoredTime(updatedAt)
	if err != nil {
		return WorkMemory{}, err
	}
	value.UpdatedAt = parsed
	return value, nil
}
