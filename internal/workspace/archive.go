package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Saieshwar5/perpetual/internal/workspaceruntime"
)

func (s *Store) migrateWorkspaceArchive(ctx context.Context) error {
	columns, err := databaseColumns(ctx, s.db, s.database.Dialect(), "workspaces")
	if err != nil {
		return err
	}
	if !columns["archived_at"] {
		if _, err := s.db.ExecContext(ctx,
			`ALTER TABLE workspaces ADD COLUMN archived_at TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("migrate workspace archive: %w", err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA user_version = 6`); err != nil {
		return fmt.Errorf("record workspace archive migration: %w", err)
	}
	return nil
}

func (s *Store) Archive(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE workspaces SET archived_at = ?, updated_at = ?
		WHERE id = ? AND archived_at = ''`, formatTime(time.Now().UTC()),
		formatTime(time.Now().UTC()), strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("archive workspace: %w", err)
	}
	return requireOneRow(result)
}

func (s *Store) Restore(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE workspaces SET archived_at = '', updated_at = ?
		WHERE id = ? AND archived_at != ''`, formatTime(time.Now().UTC()), strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("restore workspace: %w", err)
	}
	return requireOneRow(result)
}

func (s *Service) Archive(ctx context.Context, id string) error {
	value, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if value.ArchivedAt != nil {
		return errors.New("workspace is already archived")
	}
	if value.Status == StatusCreating || value.Status == StatusInitializing {
		return errors.New("workspace preparation is still running; wait before archiving it")
	}
	if value.Status == StatusDeleting || value.Status == StatusDeletionFailed {
		return errors.New("workspace deletion must finish before it can be archived")
	}
	working, err := s.store.HasWorkingSession(ctx, id)
	if err != nil {
		return fmt.Errorf("inspect running sessions: %w", err)
	}
	if working {
		return errors.New("a session is still running; stop it before archiving the workspace")
	}
	if value.Status == StatusReady {
		runtime, err := s.runtimeFor(value)
		if err != nil {
			return err
		}
		if err := runtime.Stop(ctx, runtimeRef(value)); err != nil {
			return fmt.Errorf("stop workspace runtime: %w", err)
		}
		if err := s.store.UpdateRuntimeState(ctx, id, workspaceruntime.RuntimeStateStopped); err != nil {
			return fmt.Errorf("record stopped runtime: %w", err)
		}
		if err := s.store.UpdateStatus(ctx, id, StatusStopped, ""); err != nil {
			return err
		}
	}
	return s.store.Archive(ctx, id)
}

func (s *Service) RestoreArchived(ctx context.Context, id string) error {
	value, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if value.ArchivedAt == nil {
		return sql.ErrNoRows
	}
	if value.Status == StatusDeleting || value.Status == StatusDeletionFailed {
		return errors.New("workspace deletion must finish before it can be restored")
	}
	return s.store.Restore(ctx, id)
}

func requireActiveWorkspace(value Workspace) error {
	if value.ArchivedAt != nil {
		return errors.New("workspace is archived; restore it before continuing")
	}
	return nil
}
