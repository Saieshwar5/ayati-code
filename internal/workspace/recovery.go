package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const interruptedPreparationMessage = "Workspace preparation was interrupted when Perpetual restarted"

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
		if value.Status == StatusInitializationFailed && value.Error == interruptedPreparationMessage {
			writable := value.EffectiveMountMode != "ro"
			if err := s.environment.Stop(ctx, runtimeInput(value, writable)); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("release interrupted environment for %s: %w", value.ID, err)
			}
		}
	}
	for _, value := range values {
		if value.Status != StatusReady {
			continue
		}
		if _, err := s.environment.Ensure(ctx,
			runtimeInput(value, value.Authority == AuthorityDevelop)); err != nil {
			message := "Environment unavailable after restart: " + boundedMessage(err.Error())
			if updateErr := s.store.UpdateStatus(ctx, value.ID, StatusStopped, message); updateErr != nil {
				return fmt.Errorf("record stopped workspace %s: %w", value.ID, updateErr)
			}
		}
	}
	return nil
}
