package workspace

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/sandbox"
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

func (a Authority) MountMode() sandbox.MountMode {
	if a == AuthorityDevelop {
		return sandbox.MountReadWrite
	}
	return sandbox.MountReadOnly
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
