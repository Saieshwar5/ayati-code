package workspace

import (
	"context"
	"fmt"
	"time"
)

const interruptedPreparationMessage = "Workspace preparation was interrupted when Perpetual restarted"

func (s *Store) recoverInterruptedWork(ctx context.Context) error {
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, `UPDATE workspaces SET
		status = ?, error = ?,
		preparation_failed_stage = preparation_stage,
		preparation_stage = ?, preparation_detail = ?, updated_at = ?
		WHERE status IN (?, ?)`, StatusInitializationFailed, interruptedPreparationMessage,
		PreparationFailed, "Workspace preparation was interrupted", now,
		StatusCreating, StatusInitializing)
	if err != nil {
		return fmt.Errorf("recover interrupted workspace preparation: %w", err)
	}
	return nil
}

func (s *Service) Recover(ctx context.Context) error {
	values, err := s.store.List(ctx)
	if err != nil {
		return fmt.Errorf("list workspaces for recovery: %w", err)
	}
	archived, err := s.store.ListArchived(ctx)
	if err != nil {
		return fmt.Errorf("list archived workspaces for recovery: %w", err)
	}
	for _, value := range append(values, archived...) {
		if value.Status == StatusDeleting || value.Status == StatusDeletionFailed {
			_ = s.Delete(ctx, value.ID)
		}
	}
	return nil
}
