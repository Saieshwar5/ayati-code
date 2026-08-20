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
	SessionStatusIdle     = "idle"
	SessionStatusWorking  = "working"
	SessionStatusReview   = "review"
	SessionStatusFailed   = "failed"
	SessionStatusCanceled = "canceled"
)

var sessionStatuses = map[string]bool{
	SessionStatusIdle: true, SessionStatusWorking: true,
	SessionStatusReview: true, SessionStatusFailed: true, SessionStatusCanceled: true,
}

type Session struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type sessionExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *Store) CreateSession(ctx context.Context, workspaceID, title string) (Session, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	workspace, err := s.Get(ctx, workspaceID)
	if err != nil {
		return Session{}, fmt.Errorf("load session workspace: %w", err)
	}
	if err := requireActiveWorkspace(workspace); err != nil {
		return Session{}, err
	}
	now := time.Now().UTC()
	value, err := createSession(ctx, s.db, workspaceID, title, now)
	if err != nil {
		return Session{}, err
	}
	if err := s.touchWorkspace(ctx, workspaceID, now); err != nil {
		return Session{}, err
	}
	return value, nil
}

func createSession(
	ctx context.Context, executor sessionExecutor, workspaceID, title string, now time.Time,
) (Session, error) {
	workspaceID, title = strings.TrimSpace(workspaceID), strings.TrimSpace(title)
	if workspaceID == "" {
		return Session{}, errors.New("session workspace is required")
	}
	if title == "" {
		title = "New session"
	}
	id, err := newID()
	if err != nil {
		return Session{}, err
	}
	value := Session{
		ID: id, WorkspaceID: workspaceID, Title: title, Status: SessionStatusIdle,
		CreatedAt: now, UpdatedAt: now,
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO sessions (
		id, workspace_id, title, status, error, created_at, updated_at
	) VALUES (?, ?, ?, ?, '', ?, ?)`, value.ID, value.WorkspaceID, value.Title,
		value.Status, formatTime(value.CreatedAt), formatTime(value.UpdatedAt))
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return value, nil
}

func (s *Store) GetSession(ctx context.Context, workspaceID, sessionID string) (Session, error) {
	row := s.db.QueryRowContext(ctx, selectSession+` WHERE workspace_id = ? AND id = ?`,
		strings.TrimSpace(workspaceID), strings.TrimSpace(sessionID))
	return scanSession(row)
}

func (s *Store) ListSessions(ctx context.Context, workspaceID string) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, selectSession+` WHERE workspace_id = ? ORDER BY updated_at DESC`,
		strings.TrimSpace(workspaceID))
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	var values []Session
	for rows.Next() {
		value, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) UpdateSessionStatus(ctx context.Context, id, status, message string) error {
	if !sessionStatuses[status] {
		return fmt.Errorf("invalid session status %q", status)
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET status = ?, error = ?, updated_at = ? WHERE id = ?`,
		status, strings.TrimSpace(message), formatTime(now), strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("update session: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return err
	}
	return s.touchWorkspaceForSession(ctx, id, now)
}

func (s *Store) RenameSession(ctx context.Context, workspaceID, id, title string) (Session, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Session{}, errors.New("session title is required")
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET title = ?, updated_at = ?
		WHERE workspace_id = ? AND id = ?`, title, formatTime(now),
		strings.TrimSpace(workspaceID), strings.TrimSpace(id))
	if err != nil {
		return Session{}, fmt.Errorf("rename session: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return Session{}, err
	}
	if err := s.touchWorkspace(ctx, workspaceID, now); err != nil {
		return Session{}, err
	}
	return s.GetSession(ctx, workspaceID, id)
}

func (s *Store) TitleSessionFromMessage(ctx context.Context, id, message string) error {
	title := sessionTitle(message)
	if title == "" {
		return nil
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET title = ?, updated_at = ?
		WHERE id = ? AND title = 'New session'`, title, formatTime(now), strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return nil
	}
	return s.touchWorkspaceForSession(ctx, id, now)
}

func (s *Store) DeleteSession(ctx context.Context, workspaceID, id string) error {
	value, err := s.GetSession(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	if value.Status == SessionStatusWorking {
		return errors.New("cannot delete a running session")
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE workspace_id = ?`,
		strings.TrimSpace(workspaceID)).Scan(&count); err != nil {
		return fmt.Errorf("count workspace sessions: %w", err)
	}
	if count <= 1 {
		return errors.New("a workspace must keep at least one session")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE workspace_id = ? AND id = ?`,
		strings.TrimSpace(workspaceID), strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return err
	}
	return s.touchWorkspace(ctx, workspaceID, time.Now().UTC())
}

func (s *Store) HasWorkingSession(ctx context.Context, workspaceID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions
		WHERE workspace_id = ? AND status = ?`, strings.TrimSpace(workspaceID), SessionStatusWorking).Scan(&count)
	return count != 0, err
}

func sessionTitle(message string) string {
	var line string
	for _, candidate := range strings.Split(message, "\n") {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" && candidate != "Work on it." {
			line = candidate
			break
		}
	}
	runes := []rune(line)
	if len(runes) > 60 {
		line = string(runes[:59]) + "…"
	}
	return line
}

func requireOneRow(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) touchWorkspace(ctx context.Context, workspaceID string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE workspaces SET updated_at = ? WHERE id = ?`,
		formatTime(now), strings.TrimSpace(workspaceID))
	if err != nil {
		return fmt.Errorf("touch workspace: %w", err)
	}
	return requireOneRow(result)
}

func (s *Store) touchWorkspaceForSession(ctx context.Context, sessionID string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE workspaces SET updated_at = ?
		WHERE id = (SELECT workspace_id FROM sessions WHERE id = ?)`,
		formatTime(now), strings.TrimSpace(sessionID))
	if err != nil {
		return fmt.Errorf("touch session workspace: %w", err)
	}
	return requireOneRow(result)
}

const selectSession = `SELECT id, workspace_id, title, status, error,
	created_at, updated_at FROM sessions`

func scanSession(row scanner) (Session, error) {
	var value Session
	var createdAt, updatedAt string
	if err := row.Scan(&value.ID, &value.WorkspaceID, &value.Title, &value.Status,
		&value.Error, &createdAt, &updatedAt); err != nil {
		return Session{}, err
	}
	var err error
	if value.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return Session{}, fmt.Errorf("decode session creation time: %w", err)
	}
	if value.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return Session{}, fmt.Errorf("decode session update time: %w", err)
	}
	return value, nil
}
