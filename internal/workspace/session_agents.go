package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

func (s *Store) SelectSessionAgent(
	ctx context.Context, workspaceID, id, agentID string,
) (Session, error) {
	value, err := s.GetAgent(ctx, agentID)
	if err != nil {
		return Session{}, fmt.Errorf("load selected agent: %w", err)
	}
	if value.ArchivedAt != nil {
		return Session{}, errors.New("an archived agent cannot be selected")
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET selected_agent_id = ?, updated_at = ?
		WHERE workspace_id = ? AND id = ?`, value.ID, formatTime(now),
		strings.TrimSpace(workspaceID), strings.TrimSpace(id))
	if err != nil {
		return Session{}, fmt.Errorf("select session agent: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return Session{}, err
	}
	if err := s.touchWorkspace(ctx, workspaceID, now); err != nil {
		return Session{}, err
	}
	return s.GetSession(ctx, workspaceID, id)
}

func createLegacySession(
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
		SelectedAgentID: agent.BuiltinAgentID, CreatedAt: now, UpdatedAt: now,
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO sessions (
		id, workspace_id, title, status, error, created_at, updated_at
	) VALUES (?, ?, ?, ?, '', ?, ?)`, value.ID, value.WorkspaceID, value.Title,
		value.Status, formatTime(value.CreatedAt), formatTime(value.UpdatedAt))
	if err != nil {
		return Session{}, fmt.Errorf("create legacy session: %w", err)
	}
	return value, nil
}
