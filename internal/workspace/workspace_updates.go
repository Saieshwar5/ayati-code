package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) UpdateStatus(ctx context.Context, id, status, message string) error {
	if !statuses[status] {
		return fmt.Errorf("invalid workspace status %q", status)
	}
	result, err := s.execContext(ctx,
		`UPDATE workspaces SET status = ?, error = ?, updated_at = ? WHERE id = ?`,
		status, strings.TrimSpace(message), formatTime(time.Now().UTC()), strings.TrimSpace(id),
	)
	if err != nil {
		return fmt.Errorf("update workspace: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) UpdateSetup(ctx context.Context, id, command string) error {
	result, err := s.execContext(ctx,
		`UPDATE workspaces SET setup_command = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(command), formatTime(time.Now().UTC()), strings.TrimSpace(id),
	)
	if err != nil {
		return fmt.Errorf("update workspace setup: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) UpdatePullRequest(ctx context.Context, id string, number int, pullURL string) error {
	if number < 1 || strings.TrimSpace(pullURL) == "" {
		return errors.New("pull request number and URL are required")
	}
	result, err := s.execContext(ctx, `UPDATE workspaces SET
		pull_request_number = ?, pull_request_url = ?, updated_at = ? WHERE id = ?`,
		number, strings.TrimSpace(pullURL),
		formatTime(time.Now().UTC()), strings.TrimSpace(id),
	)
	if err != nil {
		return fmt.Errorf("update pull request: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	result, err := s.execContext(ctx, `DELETE FROM workspaces WHERE id = ?`, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("delete workspace: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}
