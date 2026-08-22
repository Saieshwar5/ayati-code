package workspace

import (
	"context"
	"fmt"
)

func (s *Store) migrateWorkspaceReadiness(ctx context.Context) error {
	columns, err := databaseColumns(ctx, s.db, s.database.Dialect(), "workspaces")
	if err != nil {
		return err
	}
	statements := []string{}
	if !columns["preparation_stage"] {
		statements = append(statements,
			`ALTER TABLE workspaces ADD COLUMN preparation_stage TEXT NOT NULL DEFAULT 'pending'`)
	}
	if !columns["preparation_detail"] {
		statements = append(statements,
			`ALTER TABLE workspaces ADD COLUMN preparation_detail TEXT NOT NULL DEFAULT ''`)
	}
	if !columns["preparation_failed_stage"] {
		statements = append(statements,
			`ALTER TABLE workspaces ADD COLUMN preparation_failed_stage TEXT NOT NULL DEFAULT ''`)
	}
	if !columns["selected_project_root"] {
		statements = append(statements,
			`ALTER TABLE workspaces ADD COLUMN selected_project_root TEXT NOT NULL DEFAULT ''`)
	}
	if !columns["configuration_candidates"] {
		statements = append(statements,
			`ALTER TABLE workspaces ADD COLUMN configuration_candidates TEXT NOT NULL DEFAULT '[]'`)
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate workspace readiness: %w", err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE workspaces SET preparation_stage = CASE
		WHEN status = 'ready' THEN 'ready'
		WHEN status = 'initialization_failed' THEN 'failed'
		WHEN status = 'initializing' THEN 'analyzing'
		ELSE preparation_stage END WHERE preparation_stage = 'pending'`); err != nil {
		return fmt.Errorf("migrate workspace readiness state: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA user_version = 5`); err != nil {
		return fmt.Errorf("record workspace readiness schema: %w", err)
	}
	return nil
}
