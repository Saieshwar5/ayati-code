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

const (
	PreparationPending            = "pending"
	PreparationCloning            = "cloning"
	PreparationAnalyzing          = "analyzing"
	PreparationInstalling         = "installing"
	PreparationVerifying          = "verifying"
	PreparationSealing            = "sealing"
	PreparationNeedsConfiguration = "needs_configuration"
	PreparationReady              = "ready"
	PreparationFailed             = "failed"
)

var preparationStages = map[string]bool{
	PreparationPending: true, PreparationCloning: true, PreparationAnalyzing: true,
	PreparationInstalling: true, PreparationVerifying: true, PreparationSealing: true,
	PreparationNeedsConfiguration: true, PreparationReady: true, PreparationFailed: true,
}

type ProjectCandidate struct {
	ProjectRoot     string   `json:"project_root"`
	Languages       []string `json:"languages"`
	PackageManagers []string `json:"package_managers"`
}

func (s *Store) UpdatePreparation(ctx context.Context, id, stage, detail string) error {
	if !preparationStages[stage] {
		return fmt.Errorf("invalid preparation stage %q", stage)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE workspaces SET preparation_stage = ?,
		preparation_detail = ?, preparation_failed_stage = '', updated_at = ? WHERE id = ?`, stage, strings.TrimSpace(detail),
		formatTime(time.Now().UTC()), strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("update workspace preparation: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) RequireProjectSelection(
	ctx context.Context, id string, candidates []ProjectCandidate,
) error {
	encoded, err := json.Marshal(nonNilCandidates(candidates))
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE workspaces SET status = ?, error = '',
		preparation_stage = ?, preparation_detail = ?, configuration_candidates = ?,
		preparation_failed_stage = '', selected_project_root = '', updated_at = ? WHERE id = ?`,
		StatusNeedsConfiguration, PreparationNeedsConfiguration,
		"Choose the project Ayati should prepare", string(encoded), formatTime(time.Now().UTC()), id)
	if err != nil {
		return fmt.Errorf("record project selection: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SelectProjectRoot(ctx context.Context, id, root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return errors.New("project root is required")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE workspaces SET selected_project_root = ?,
		status = ?, error = '', preparation_stage = ?, preparation_detail = '',
		preparation_failed_stage = '', updated_at = ?
		WHERE id = ?`, root, StatusCreating, PreparationPending, formatTime(time.Now().UTC()), id)
	if err != nil {
		return fmt.Errorf("select project root: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) FailPreparation(ctx context.Context, id, message string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE workspaces SET status = ?, error = ?,
		preparation_failed_stage = preparation_stage, preparation_stage = ?,
		preparation_detail = ?, updated_at = ? WHERE id = ?`,
		StatusInitializationFailed, strings.TrimSpace(message), PreparationFailed,
		"Workspace preparation failed", formatTime(time.Now().UTC()), strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("record workspace preparation failure: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) CompletePreparation(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE workspaces SET status = ?, error = '',
		preparation_stage = ?, preparation_detail = ?, preparation_failed_stage = '',
		configuration_candidates = '[]',
		updated_at = ? WHERE id = ?`, StatusReady, PreparationReady, "Workspace ready",
		formatTime(time.Now().UTC()), strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("complete workspace preparation: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func decodeProjectCandidates(value string) ([]ProjectCandidate, error) {
	var candidates []ProjectCandidate
	if err := json.Unmarshal([]byte(value), &candidates); err != nil {
		return nil, fmt.Errorf("decode project candidates: %w", err)
	}
	return nonNilCandidates(candidates), nil
}

func nonNilCandidates(values []ProjectCandidate) []ProjectCandidate {
	if values == nil {
		return []ProjectCandidate{}
	}
	for index := range values {
		values[index].Languages = nonNilStrings(values[index].Languages)
		values[index].PackageManagers = nonNilStrings(values[index].PackageManagers)
	}
	return values
}
