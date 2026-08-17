package workspace

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Authority string

const (
	AuthorityExplore Authority = "explore"
	AuthorityDevelop Authority = "develop"
)

func ParseAuthority(value string) (Authority, error) {
	authority := Authority(strings.ToLower(strings.TrimSpace(value)))
	if authority == "" {
		return AuthorityExplore, nil
	}
	if !authority.Valid() {
		return "", fmt.Errorf("invalid workspace authority %q", value)
	}
	return authority, nil
}

func (a Authority) Valid() bool {
	return a == AuthorityExplore || a == AuthorityDevelop
}

func (a Authority) MountMode() string {
	if a == AuthorityDevelop {
		return "rw"
	}
	return "ro"
}

func (s *Store) UpdateEffectiveMountMode(ctx context.Context, id, mode string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE workspaces SET effective_mount_mode = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(mode), formatTime(time.Now().UTC()), strings.TrimSpace(id),
	)
	if err != nil {
		return fmt.Errorf("update workspace mount mode: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) CompleteAuthorityChange(
	ctx context.Context, id string, authority Authority, branch string, createBranch bool, mode string,
) error {
	result, err := s.db.ExecContext(ctx, `UPDATE workspaces SET authority = ?, branch = ?,
		create_branch = ?, effective_mount_mode = ?, status = ?, error = '',
		preparation_stage = ?, preparation_detail = ?, preparation_failed_stage = '', updated_at = ?
		WHERE id = ?`, authority, strings.TrimSpace(branch), createBranch, strings.TrimSpace(mode),
		StatusReady, PreparationReady, "Workspace ready", formatTime(time.Now().UTC()), strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("complete workspace authority change: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) RestoreAuthorityAfterFailure(ctx context.Context, id, mode string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE workspaces SET effective_mount_mode = ?,
		status = ?, error = '', preparation_stage = ?, preparation_detail = ?,
		preparation_failed_stage = '', updated_at = ? WHERE id = ?`, strings.TrimSpace(mode),
		StatusReady, PreparationReady, "Workspace ready",
		formatTime(time.Now().UTC()), strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("restore workspace authority: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}
