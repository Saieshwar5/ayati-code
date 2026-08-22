package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const projectEnvironmentsOwnerSchema = `CREATE TABLE project_environments_v2 (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL DEFAULT '',
	repository TEXT NOT NULL,
	project_root TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE(user_id, repository, project_root)
)`

func (s *Store) migrateEnvironmentVersions(ctx context.Context) error {
	for _, statement := range []string{projectEnvironmentsSchema, environmentVersionsSchema} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create environment version schema: %w", err)
		}
	}
	if err := s.migrateProjectEnvironmentOwner(ctx); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS environment_versions_lookup
		ON environment_versions(environment_id, source_fingerprint, state)`); err != nil {
		return fmt.Errorf("create environment version index: %w", err)
	}
	columns, err := databaseColumns(ctx, s.db, s.database.Dialect(), "workspaces")
	if err != nil {
		return err
	}
	if !columns["environment_version_id"] {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE workspaces ADD COLUMN
			environment_version_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("migrate workspace environment binding: %w", err)
		}
	}
	if err := s.migrateEnvironmentSnapshotColumns(ctx); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA user_version = 14`); err != nil {
		return fmt.Errorf("record environment version schema: %w", err)
	}
	return s.backfillEnvironmentVersions(ctx)
}

func (s *Store) migrateEnvironmentSnapshotColumns(ctx context.Context) error {
	columns, err := databaseColumns(ctx, s.db, s.database.Dialect(), "environment_versions")
	if err != nil {
		return err
	}
	for _, statement := range []string{
		`ALTER TABLE environment_versions ADD COLUMN snapshot_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE environment_versions ADD COLUMN snapshot_ref TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE environment_versions ADD COLUMN snapshot_manifest TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE environment_versions ADD COLUMN snapshot_bytes INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE environment_versions ADD COLUMN snapshot_created_at TEXT NOT NULL DEFAULT ''`,
	} {
		name := strings.TrimSpace(strings.TrimPrefix(statement, "ALTER TABLE environment_versions ADD COLUMN "))
		name = strings.Fields(name)[0]
		if columns[name] {
			continue
		}
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate environment version %s: %w", name, err)
		}
	}
	return nil
}

func (s *Store) migrateProjectEnvironmentOwner(ctx context.Context) error {
	columns, err := databaseColumns(ctx, s.db, s.database.Dialect(), "project_environments")
	if err != nil {
		return err
	}
	if columns["user_id"] {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("suspend foreign keys for environment owner migration: %w", err)
	}
	defer func() { _, _ = s.db.Exec(`PRAGMA foreign_keys = ON`) }()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin environment owner migration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, projectEnvironmentsOwnerSchema); err != nil {
		return fmt.Errorf("create owner-scoped project environments: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO project_environments_v2
		(id, user_id, repository, project_root, created_at, updated_at)
		SELECT id, '', repository, project_root, created_at, updated_at
		FROM project_environments`); err != nil {
		return fmt.Errorf("copy project environments with owner: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE project_environments`); err != nil {
		return fmt.Errorf("remove legacy project environments: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE project_environments_v2 RENAME TO project_environments`); err != nil {
		return fmt.Errorf("install owner-scoped project environments: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS project_environments_owner
		ON project_environments(user_id, repository, project_root)`); err != nil {
		return fmt.Errorf("create project environment owner index: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit environment owner migration: %w", err)
	}
	return nil
}

func (s *Store) backfillEnvironmentVersions(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT w.id, w.user_id, w.repository, wp.project_root,
		wp.environment_spec, wp.cache_path FROM workspace_profiles wp
		JOIN workspaces w ON w.id = wp.workspace_id
		WHERE wp.environment_spec != ''`)
	if err != nil {
		return fmt.Errorf("find profiles for environment backfill: %w", err)
	}
	type backfillRow struct {
		workspaceID string
		userID      string
		repository  string
		projectRoot string
		spec        EnvironmentSpec
		cacheRef    string
	}
	var values []backfillRow
	for rows.Next() {
		var workspaceID, userID, repository, projectRoot, specJSON, cacheRef string
		if err := rows.Scan(&workspaceID, &userID, &repository, &projectRoot, &specJSON, &cacheRef); err != nil {
			rows.Close()
			return fmt.Errorf("scan environment backfill profile: %w", err)
		}
		var spec EnvironmentSpec
		if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
			rows.Close()
			return fmt.Errorf("decode environment backfill spec: %w", err)
		}
		values = append(values, backfillRow{
			workspaceID: workspaceID, userID: userID, repository: repository,
			projectRoot: projectRoot, spec: spec, cacheRef: cacheRef,
		})
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, value := range values {
		environment, err := s.FindOrCreateEnvironment(ctx, value.userID, value.repository, value.projectRoot)
		if err != nil {
			return err
		}
		var version EnvironmentVersion
		version, found, err := s.FindReadyEnvironmentVersion(ctx, value.userID, environment.ID, value.spec.Fingerprint)
		if err != nil {
			return err
		}
		if found {
		} else {
			version, err = s.CreateEnvironmentVersion(ctx, environment.ID,
				value.spec.Fingerprint, value.spec, value.cacheRef)
			if err != nil {
				return err
			}
			if err := s.SetEnvironmentVersionState(ctx, version.ID, EnvironmentVersionReady, ""); err != nil {
				return err
			}
		}
		if err := s.BindWorkspaceEnvironment(ctx, value.workspaceID, version.ID); err != nil {
			return err
		}
	}
	return nil
}
