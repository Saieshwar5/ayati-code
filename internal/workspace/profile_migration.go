package workspace

import (
	"context"
	"fmt"
)

const projectProfileSchema = `CREATE TABLE IF NOT EXISTS workspace_profiles (
	workspace_id TEXT PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
	project_root TEXT NOT NULL,
	languages TEXT NOT NULL,
	runtime_versions TEXT NOT NULL,
	package_managers TEXT NOT NULL,
	lockfiles TEXT NOT NULL,
	setup_command TEXT NOT NULL,
	test_command TEXT NOT NULL,
	lint_command TEXT NOT NULL,
	typecheck_command TEXT NOT NULL,
	build_command TEXT NOT NULL,
	instructions_file TEXT NOT NULL,
	manifest_fingerprint TEXT NOT NULL,
	baseline_commit TEXT NOT NULL,
	setup_result TEXT NOT NULL,
	baseline_result TEXT NOT NULL,
	cache_path TEXT NOT NULL,
	environment_spec TEXT NOT NULL DEFAULT '',
	prepared_at TEXT NOT NULL
)`

func (s *Store) migrateProjectProfiles(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, projectProfileSchema); err != nil {
		return fmt.Errorf("create workspace profiles: %w", err)
	}
	columns, err := databaseColumns(ctx, s.db, s.database.Dialect(), "workspace_profiles")
	if err != nil {
		return err
	}
	if !columns["environment_spec"] {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE workspace_profiles
			ADD COLUMN environment_spec TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("migrate workspace profile environment spec: %w", err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA user_version = 4`); err != nil {
		return fmt.Errorf("record workspace profile schema: %w", err)
	}
	return nil
}
