package workspace

import (
	"context"
	"fmt"
	"time"
)

const interruptedPreparationMessage = "Workspace preparation was interrupted when Ayati restarted"

func (s *Store) recoverInterruptedWork(ctx context.Context) error {
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, `UPDATE workspaces SET
		status = ?, error = ?, effective_mount_mode = '',
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
	for _, value := range values {
		if value.Status != StatusInitializationFailed || value.Error != interruptedPreparationMessage {
			continue
		}
		if err := s.environment.Remove(ctx, value.SandboxName); err != nil {
			return fmt.Errorf("remove interrupted sandbox %s: %w", value.SandboxName, err)
		}
	}
	return nil
}
