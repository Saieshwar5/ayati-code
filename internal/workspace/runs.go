package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Execution run states. These describe one execution-room lifecycle.
const (
	RunQueued      = "queued"
	RunRunning     = "running"
	RunWaitingUser = "waiting_user"
	RunCompleted   = "completed"
	RunFailed      = "failed"
	RunCanceled    = "canceled"
)

// Run step kinds.
const (
	StepModel   = "model"
	StepShell   = "shell"
	StepCompact = "compact"
	StepPause   = "pause"
)

const runLeaseOwner = "execution-worker"
const runLeaseDuration = 60 * time.Second

// Run is one durable execution room. It owns the agent loop's state machine.
type Run struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	WorkspaceID    string    `json:"workspace_id"`
	SessionID      string    `json:"session_id"`
	State          string    `json:"state"`
	StepCursor     int64     `json:"step_cursor"`
	MaxSteps       int64     `json:"max_steps"`
	Prompt         string    `json:"prompt,omitempty"`
	DeadlineAt     time.Time `json:"deadline_at"`
	LeaseOwner     string    `json:"lease_owner,omitempty"`
	LeaseExpiresAt time.Time `json:"lease_expires_at,omitempty"`
	HeartbeatAt    time.Time `json:"heartbeat_at,omitempty"`
	CurrentCommand string    `json:"current_command,omitempty"`
	Result         string    `json:"result,omitempty"`
	Error          string    `json:"error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	FinishedAt     time.Time `json:"finished_at,omitempty"`
}

// RunStep is one idempotent unit of durable work inside a run.
type RunStep struct {
	RunID     string         `json:"run_id"`
	StepKey   string         `json:"step_key"`
	Kind      string         `json:"kind"`
	Status    string         `json:"status"`
	Input     map[string]any `json:"input,omitempty"`
	Output    map[string]any `json:"output,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	DoneAt    time.Time      `json:"done_at,omitempty"`
}

// WorkMemory is the compact per-run scratchpad the agent actively updates.
type WorkMemory struct {
	RunID     string         `json:"run_id"`
	Notes     map[string]any `json:"notes"`
	Version   int64          `json:"version"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// EnqueueRunInput is what the controller needs to create a queued execution
// room. Context window and compaction budgets are carried so the loop never
// needs a second lookup to check overflow.
type EnqueueRunInput struct {
	UserID      string
	WorkspaceID string
	SessionID   string
	Prompt      string
	MaxSteps    int64
	Deadline    time.Time
}

const runsSchema = `CREATE TABLE IF NOT EXISTS agent_runs (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
	state TEXT NOT NULL,
	step_cursor INTEGER NOT NULL DEFAULT 0,
	max_steps INTEGER NOT NULL DEFAULT 200,
	prompt TEXT NOT NULL DEFAULT '',
	deadline_at TEXT NOT NULL DEFAULT '',
	lease_owner TEXT NOT NULL DEFAULT '',
	lease_expires_at TEXT NOT NULL DEFAULT '',
	heartbeat_at TEXT NOT NULL DEFAULT '',
	current_command TEXT NOT NULL DEFAULT '',
	result TEXT NOT NULL DEFAULT '',
	error TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	started_at TEXT NOT NULL DEFAULT '',
	finished_at TEXT NOT NULL DEFAULT ''
)`

const runStepsSchema = `CREATE TABLE IF NOT EXISTS agent_run_steps (
	run_id TEXT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
	step_key TEXT NOT NULL,
	kind TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	input TEXT NOT NULL DEFAULT '{}',
	output TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL,
	done_at TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (run_id, step_key)
)`

const workMemorySchema = `CREATE TABLE IF NOT EXISTS agent_work_memory (
	run_id TEXT PRIMARY KEY REFERENCES agent_runs(id) ON DELETE CASCADE,
	notes TEXT NOT NULL DEFAULT '{}',
	version INTEGER NOT NULL DEFAULT 0,
	updated_at TEXT NOT NULL
)`

// createRunTables installs the execution-room schema. It is called from the
// store configure path, so fresh SQLite and Postgres databases both work.
func (s *Store) createRunTables() error {
	for _, statement := range []string{
		runsSchema,
		`CREATE INDEX IF NOT EXISTS agent_runs_claim ON agent_runs(state, created_at)`,
		`CREATE INDEX IF NOT EXISTS agent_runs_user ON agent_runs(user_id, state)`,
		`CREATE INDEX IF NOT EXISTS agent_runs_workspace ON agent_runs(workspace_id, state)`,
		`CREATE INDEX IF NOT EXISTS agent_runs_session ON agent_runs(session_id)`,
		runStepsSchema,
		workMemorySchema,
	} {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("create execution room schema: %w", err)
		}
	}
	return nil
}

// EnqueueRun persists a queued execution room.
func (s *Store) EnqueueRun(ctx context.Context, input EnqueueRunInput) (Run, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.SessionID = strings.TrimSpace(input.SessionID)
	if input.UserID == "" || input.WorkspaceID == "" || input.SessionID == "" {
		return Run{}, errors.New("run user, workspace, and session are required")
	}
	if input.MaxSteps <= 0 {
		input.MaxSteps = 200
	}
	if input.Deadline.IsZero() {
		input.Deadline = time.Now().UTC().Add(2 * time.Hour)
	}
	id, err := newID()
	if err != nil {
		return Run{}, err
	}
	now := time.Now().UTC()
	value := Run{
		ID: id, UserID: input.UserID, WorkspaceID: input.WorkspaceID,
		SessionID: input.SessionID, State: RunQueued, MaxSteps: input.MaxSteps,
		Prompt:     strings.TrimSpace(input.Prompt),
		DeadlineAt: input.Deadline, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := s.execContext(ctx, `INSERT INTO agent_runs (
		id, user_id, workspace_id, session_id, state, step_cursor, max_steps,
		prompt, deadline_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?)`,
		value.ID, value.UserID, value.WorkspaceID, value.SessionID, value.State,
		value.MaxSteps, strings.TrimSpace(input.Prompt), formatTime(value.DeadlineAt),
		formatTime(value.CreatedAt), formatTime(value.UpdatedAt)); err != nil {
		return Run{}, fmt.Errorf("enqueue execution room: %w", err)
	}
	return value, nil
}

// GetRun returns one run by ID.
func (s *Store) GetRun(ctx context.Context, id string) (Run, error) {
	row := s.queryRowContext(ctx, `SELECT id, user_id, workspace_id, session_id, state,
		step_cursor, max_steps, prompt, deadline_at, lease_owner, lease_expires_at,
		heartbeat_at, current_command, result, error, created_at, updated_at,
		started_at, finished_at FROM agent_runs WHERE id = ?`, strings.TrimSpace(id))
	return scanRun(row)
}

// RunsForWorkspace lists runs for one workspace ordered newest first.
func (s *Store) RunsForWorkspace(ctx context.Context, workspaceID string) ([]Run, error) {
	rows, err := s.queryContext(ctx, `SELECT id, user_id, workspace_id, session_id, state,
		step_cursor, max_steps, prompt, deadline_at, lease_owner, lease_expires_at,
		heartbeat_at, current_command, result, error, created_at, updated_at,
		started_at, finished_at FROM agent_runs WHERE workspace_id = ?
		ORDER BY created_at DESC`, strings.TrimSpace(workspaceID))
	if err != nil {
		return nil, fmt.Errorf("list execution rooms: %w", err)
	}
	defer rows.Close()
	var values []Run
	for rows.Next() {
		value, err := scanRunRow(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func scanRun(row *sql.Row) (Run, error) {
	return scanRunRow(row)
}

func scanRunRow(row scanner) (Run, error) {
	var value Run
	var deadlineAt, leaseExpiresAt, heartbeatAt, createdAt, updatedAt, startedAt, finishedAt string
	err := row.Scan(&value.ID, &value.UserID, &value.WorkspaceID, &value.SessionID,
		&value.State, &value.StepCursor, &value.MaxSteps, &value.Prompt, &deadlineAt,
		&value.LeaseOwner, &leaseExpiresAt, &heartbeatAt, &value.CurrentCommand,
		&value.Result, &value.Error, &createdAt, &updatedAt, &startedAt, &finishedAt)
	if err != nil {
		return Run{}, err
	}
	value.DeadlineAt, err = parseStoredTimeOrZero(deadlineAt, time.Time{})
	if err != nil {
		return Run{}, err
	}
	value.LeaseExpiresAt, err = parseStoredTimeOrZero(leaseExpiresAt, time.Time{})
	if err != nil {
		return Run{}, err
	}
	value.HeartbeatAt, err = parseStoredTimeOrZero(heartbeatAt, time.Time{})
	if err != nil {
		return Run{}, err
	}
	value.CreatedAt, err = parseStoredTime(createdAt)
	if err != nil {
		return Run{}, err
	}
	value.UpdatedAt, err = parseStoredTime(updatedAt)
	if err != nil {
		return Run{}, err
	}
	value.StartedAt, err = parseStoredTimeOrZero(startedAt, time.Time{})
	if err != nil {
		return Run{}, err
	}
	value.FinishedAt, err = parseStoredTimeOrZero(finishedAt, time.Time{})
	if err != nil {
		return Run{}, err
	}
	return value, nil
}

func parseStoredTimeOrZero(value string, fallback time.Time) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	return parseStoredTime(value)
}

// AppendRunStep persists one idempotent step. Existing completed steps are
// returned as no-ops so replay never double executes.
func (s *Store) AppendRunStep(ctx context.Context, runID, stepKey, kind, status string, input, output map[string]any) error {
	runID, stepKey = strings.TrimSpace(runID), strings.TrimSpace(stepKey)
	if runID == "" || stepKey == "" {
		return errors.New("run ID and step key are required")
	}
	if status == "" {
		status = "done"
	}
	encodedInput, err := json.Marshal(nonNilMap(input))
	if err != nil {
		return fmt.Errorf("encode run step input: %w", err)
	}
	encodedOutput, err := json.Marshal(nonNilMap(output))
	if err != nil {
		return fmt.Errorf("encode run step output: %w", err)
	}
	now := formatTime(time.Now().UTC())
	doneAt := now
	if status == "pending" || status == "running" {
		doneAt = ""
	}
	_, err = s.execContext(ctx, `INSERT INTO agent_run_steps (
		run_id, step_key, kind, status, input, output, created_at, done_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(run_id, step_key) DO UPDATE SET
		kind = excluded.kind,
		status = excluded.status,
		output = excluded.output,
		done_at = excluded.done_at`,
		runID, stepKey, kind, status, string(encodedInput), string(encodedOutput),
		now, doneAt)
	if err != nil {
		return fmt.Errorf("append run step: %w", err)
	}
	return s.touchRun(ctx, runID, time.Now().UTC())
}

func (s *Store) touchRun(ctx context.Context, runID string, now time.Time) error {
	_, err := s.execContext(ctx, `UPDATE agent_runs SET updated_at = ? WHERE id = ?`,
		formatTime(now), runID)
	if err != nil {
		return fmt.Errorf("touch execution room: %w", err)
	}
	return nil
}

func nonNilMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}
