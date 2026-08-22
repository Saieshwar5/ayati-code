package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// LegacyClaim reports how many pre-tenancy rows were assigned to the first
// authenticated user. It is only safe to run from the first-login path; hosted
// deployments should not auto-claim rows.
type LegacyClaim struct {
	Workspaces   int64 `json:"workspaces"`
	Jobs         int64 `json:"jobs"`
	Environments int64 `json:"environments"`
}

// ClaimLegacyRows assigns rows that predate user ownership to userID. Workspace
// rows without an owner are claimed directly; jobs and environments are claimed
// when their parent workspace or matching profile belongs to that user. The
// operation is idempotent and safe to call again after a partial run.
func (s *Store) ClaimLegacyRows(ctx context.Context, userID string) (LegacyClaim, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return LegacyClaim{}, errors.New("user ID is required to claim legacy rows")
	}
	var claim LegacyClaim
	if err := withLegacyClaim(ctx, s, userID, &claim); err != nil {
		return LegacyClaim{}, err
	}
	return claim, nil
}

func withLegacyClaim(ctx context.Context, s *Store, userID string, claim *LegacyClaim) error {
	workspaces, err := s.execContext(ctx, `UPDATE workspaces SET user_id = ?
		WHERE user_id = ''`, userID)
	if err != nil {
		return fmt.Errorf("claim legacy workspaces: %w", err)
	}
	if affected, err := workspaces.RowsAffected(); err == nil {
		claim.Workspaces = affected
	}

	jobs, err := s.execContext(ctx, `UPDATE workspace_jobs SET user_id = ?
		WHERE user_id = ''
		AND EXISTS (SELECT 1 FROM workspaces WHERE workspaces.id = workspace_jobs.workspace_id
			AND workspaces.user_id = ?)`, userID, userID)
	if err != nil {
		return fmt.Errorf("claim legacy jobs: %w", err)
	}
	if affected, err := jobs.RowsAffected(); err == nil {
		claim.Jobs = affected
	}

	environments, err := s.execContext(ctx, `UPDATE project_environments SET user_id = ?
		WHERE user_id = ''
		AND EXISTS (SELECT 1 FROM workspaces w
			JOIN workspace_profiles wp ON wp.workspace_id = w.id
			WHERE w.user_id = ? AND w.repository = project_environments.repository
			AND wp.project_root = project_environments.project_root)`, userID, userID)
	if err != nil {
		return fmt.Errorf("claim legacy environments: %w", err)
	}
	if affected, err := environments.RowsAffected(); err == nil {
		claim.Environments = affected
	}
	return nil
}
