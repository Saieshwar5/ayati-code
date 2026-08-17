package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

const (
	AgentRunStatusAccepted    = "accepted"
	AgentRunStatusRunning     = "running"
	AgentRunStatusCompleted   = "completed"
	AgentRunStatusFailed      = "failed"
	AgentRunStatusCanceled    = "canceled"
	AgentRunStatusInterrupted = "interrupted"
)

var ErrAgentRunActive = errors.New("another session is already running in this workspace")

type AgentRun struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	SessionID   string     `json:"session_id"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (s *Store) BeginAgentRun(
	ctx context.Context, workspaceID, sessionID, input string,
) (AgentRun, error) {
	workspaceID, sessionID, input = strings.TrimSpace(workspaceID), strings.TrimSpace(sessionID), strings.TrimSpace(input)
	if workspaceID == "" || sessionID == "" || input == "" {
		return AgentRun{}, errors.New("workspace, session, and message are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AgentRun{}, fmt.Errorf("begin agent run: %w", err)
	}
	defer tx.Rollback()
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_runs
		WHERE workspace_id = ? AND status IN ('accepted', 'running')`, workspaceID).Scan(&active); err != nil {
		return AgentRun{}, fmt.Errorf("check active agent run: %w", err)
	}
	if active != 0 {
		return AgentRun{}, ErrAgentRunActive
	}
	var currentTitle string
	if err := tx.QueryRowContext(ctx, `SELECT title FROM sessions WHERE workspace_id = ? AND id = ?`,
		workspaceID, sessionID).Scan(&currentTitle); err != nil {
		return AgentRun{}, fmt.Errorf("load agent run session: %w", err)
	}
	id, err := newID()
	if err != nil {
		return AgentRun{}, err
	}
	now := time.Now().UTC()
	value := AgentRun{
		ID: id, WorkspaceID: workspaceID, SessionID: sessionID,
		Status: AgentRunStatusAccepted, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_runs (
		id, workspace_id, session_id, input, status, error, created_at, started_at, finished_at, updated_at
	) VALUES (?, ?, ?, ?, ?, '', ?, '', '', ?)`, id, workspaceID, sessionID, input,
		value.Status, formatTime(now), formatTime(now)); err != nil {
		return AgentRun{}, fmt.Errorf("create agent run: %w", err)
	}
	payload, err := json.Marshal(agent.Message{Role: "user", Content: input})
	if err != nil {
		return AgentRun{}, fmt.Errorf("encode agent run message: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO messages (
		session_id, payload, created_at
	) VALUES (?, ?, ?)`, sessionID, string(payload), formatTime(now)); err != nil {
		return AgentRun{}, fmt.Errorf("record agent run message: %w", err)
	}
	title := currentTitle
	if currentTitle == "New session" {
		title = sessionTitle(input)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET title = ?, status = ?, error = '', updated_at = ?
		WHERE workspace_id = ? AND id = ?`, title, SessionStatusWorking, formatTime(now), workspaceID, sessionID); err != nil {
		return AgentRun{}, fmt.Errorf("activate agent run session: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workspaces SET updated_at = ? WHERE id = ?`,
		formatTime(now), workspaceID); err != nil {
		return AgentRun{}, fmt.Errorf("touch agent run workspace: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return AgentRun{}, fmt.Errorf("commit agent run: %w", err)
	}
	return value, nil
}

func (s *Store) MarkAgentRunRunning(ctx context.Context, runID string) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE agent_runs SET status = ?, started_at = ?, updated_at = ?
		WHERE id = ? AND status = ?`, AgentRunStatusRunning, formatTime(now), formatTime(now),
		strings.TrimSpace(runID), AgentRunStatusAccepted)
	if err != nil {
		return fmt.Errorf("start agent run: %w", err)
	}
	return requireOneRow(result)
}

func (s *Store) FinishAgentRun(
	ctx context.Context, runID, runStatus, sessionStatus, message string,
) error {
	if !terminalAgentRunStatus(runStatus) || !sessionStatuses[sessionStatus] {
		return errors.New("invalid final agent run status")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin agent run completion: %w", err)
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE agent_runs SET status = ?, error = ?, finished_at = ?, updated_at = ?
		WHERE id = ? AND status IN ('accepted', 'running')`, runStatus, strings.TrimSpace(message),
		formatTime(now), formatTime(now), strings.TrimSpace(runID))
	if err != nil {
		return fmt.Errorf("finish agent run: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return err
	}
	result, err = tx.ExecContext(ctx, `UPDATE sessions SET status = ?, error = ?, updated_at = ?
		WHERE id = (SELECT session_id FROM agent_runs WHERE id = ?)`, sessionStatus,
		strings.TrimSpace(message), formatTime(now), strings.TrimSpace(runID))
	if err != nil {
		return fmt.Errorf("finish agent run session: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workspaces SET updated_at = ?
		WHERE id = (SELECT workspace_id FROM agent_runs WHERE id = ?)`, formatTime(now), runID); err != nil {
		return fmt.Errorf("touch completed agent run workspace: %w", err)
	}
	return tx.Commit()
}

func terminalAgentRunStatus(status string) bool {
	switch status {
	case AgentRunStatusCompleted, AgentRunStatusFailed, AgentRunStatusCanceled, AgentRunStatusInterrupted:
		return true
	default:
		return false
	}
}

func (s *Store) AgentRun(ctx context.Context, workspaceID, sessionID, runID string) (AgentRun, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, workspace_id, session_id, status, error,
		created_at, started_at, finished_at, updated_at FROM agent_runs
		WHERE workspace_id = ? AND session_id = ? AND id = ?`, strings.TrimSpace(workspaceID),
		strings.TrimSpace(sessionID), strings.TrimSpace(runID))
	return scanAgentRun(row)
}

func scanAgentRun(row scanner) (AgentRun, error) {
	var value AgentRun
	var createdAt, startedAt, finishedAt, updatedAt string
	if err := row.Scan(&value.ID, &value.WorkspaceID, &value.SessionID, &value.Status,
		&value.Error, &createdAt, &startedAt, &finishedAt, &updatedAt); err != nil {
		return AgentRun{}, err
	}
	var err error
	if value.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return AgentRun{}, err
	}
	if value.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return AgentRun{}, err
	}
	if value.StartedAt, err = parseOptionalRunTime(startedAt); err != nil {
		return AgentRun{}, err
	}
	if value.FinishedAt, err = parseOptionalRunTime(finishedAt); err != nil {
		return AgentRun{}, err
	}
	return value, nil
}

func parseOptionalRunTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return &parsed, err
}
