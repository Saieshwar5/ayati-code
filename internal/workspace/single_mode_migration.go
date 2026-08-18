package workspace

import (
	"context"
	"fmt"
)

func (s *Store) migrateSingleWorkspaceMode(ctx context.Context) error {
	columns, err := databaseColumns(ctx, s.db, "workspaces")
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin single workspace mode migration: %w", err)
	}
	defer tx.Rollback()
	for _, column := range []string{"authority", "effective_mount_mode"} {
		if !columns[column] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `ALTER TABLE workspaces DROP COLUMN `+column); err != nil {
			return fmt.Errorf("remove workspace %s: %w", column, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `PRAGMA user_version = 10`); err != nil {
		return fmt.Errorf("record single workspace mode migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit single workspace mode migration: %w", err)
	}
	return nil
}
