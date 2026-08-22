package workspace

import (
	"context"
	"fmt"
)

// workspacesWithoutSandbox is the current workspace table shape after the
// Docker sandbox backend was removed.
const workspacesWithoutSandbox = `CREATE TABLE workspaces_v11 (
	id TEXT PRIMARY KEY,
	repository TEXT NOT NULL,
	clone_url TEXT NOT NULL,
	base_branch TEXT NOT NULL,
	branch TEXT NOT NULL,
	create_branch INTEGER NOT NULL,
	preparation_stage TEXT NOT NULL DEFAULT 'pending',
	preparation_detail TEXT NOT NULL DEFAULT '',
	preparation_failed_stage TEXT NOT NULL DEFAULT '',
	selected_project_root TEXT NOT NULL DEFAULT '',
	configuration_candidates TEXT NOT NULL DEFAULT '[]',
	setup_command TEXT NOT NULL,
	path TEXT NOT NULL UNIQUE,
	status TEXT NOT NULL,
	error TEXT NOT NULL DEFAULT '',
	pull_request_number INTEGER NOT NULL DEFAULT 0,
	pull_request_url TEXT NOT NULL DEFAULT '',
	archived_at TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
)`

const workspaceColumnsWithoutSandbox = `id, repository, clone_url, base_branch, branch,
	create_branch, preparation_stage, preparation_detail, preparation_failed_stage,
	selected_project_root, configuration_candidates, setup_command, path, status, error,
	pull_request_number, pull_request_url, archived_at, created_at, updated_at`

// migrateRemoveComputeSandbox removes the reusable Docker environment schema and
// drops the legacy workspaces.sandbox_name column. Fresh databases skip the
// rebuild; existing sandbox-era databases are migrated without losing records.
func (s *Store) migrateRemoveComputeSandbox(ctx context.Context) error {
	columns, err := databaseColumns(ctx, s.db, s.database.Dialect(), "workspaces")
	if err != nil {
		return err
	}
	for _, statement := range []string{
		`DROP TRIGGER IF EXISTS environments_prevent_active_delete`,
		`DROP TRIGGER IF EXISTS workspaces_prevent_active_lease_delete`,
		`DROP TABLE IF EXISTS environment_leases`,
		`DROP TABLE IF EXISTS environments`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("remove compute environment schema: %w", err)
		}
	}
	if !columns["sandbox_name"] {
		if _, err := s.db.ExecContext(ctx, `PRAGMA user_version = 11`); err != nil {
			return fmt.Errorf("record compute environment removal: %w", err)
		}
		return nil
	}
	// The rebuild drops and recreates the parent workspaces table. Foreign-key
	// cascades would delete child sessions and messages, so enforce the rebuild
	// with constraints temporarily suspended on this single-threaded connection.
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("suspend foreign keys for workspace rebuild: %w", err)
	}
	defer func() { _, _ = s.db.Exec(`PRAGMA foreign_keys = ON`) }()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin compute environment removal: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, workspacesWithoutSandbox); err != nil {
		return fmt.Errorf("create workspace table without sandbox: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspaces_v11 (`+workspaceColumnsWithoutSandbox+`)
		SELECT `+workspaceColumnsWithoutSandbox+` FROM workspaces`); err != nil {
		return fmt.Errorf("copy workspaces without sandbox: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE workspaces`); err != nil {
		return fmt.Errorf("remove legacy workspaces table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE workspaces_v11 RENAME TO workspaces`); err != nil {
		return fmt.Errorf("install workspace table without sandbox: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `PRAGMA user_version = 11`); err != nil {
		return fmt.Errorf("record compute environment removal: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit compute environment removal: %w", err)
	}
	return nil
}
