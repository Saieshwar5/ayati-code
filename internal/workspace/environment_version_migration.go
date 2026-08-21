package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (s *Store) migrateEnvironmentVersions(ctx context.Context) error {
	for _, statement := range []string{projectEnvironmentsSchema, environmentVersionsSchema} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create environment version schema: %w", err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS environment_versions_lookup
		ON environment_versions(environment_id, source_fingerprint, state)`); err != nil {
		return fmt.Errorf("create environment version index: %w", err)
	}
	columns, err := databaseColumns(ctx, s.db, "workspaces")
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
	columns, err := databaseColumns(ctx, s.db, "environment_versions")
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

func (s *Store) backfillEnvironmentVersions(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT w.id, w.repository, wp.project_root,
		wp.environment_spec, wp.cache_path FROM workspace_profiles wp
		JOIN workspaces w ON w.id = wp.workspace_id
		WHERE wp.environment_spec != ''`)
	if err != nil {
		return fmt.Errorf("find profiles for environment backfill: %w", err)
	}
	type backfillRow struct {
		workspaceID string
		repository  string
		projectRoot string
		spec        EnvironmentSpec
		cacheRef    string
	}
	var values []backfillRow
	for rows.Next() {
		var workspaceID, repository, projectRoot, specJSON, cacheRef string
		if err := rows.Scan(&workspaceID, &repository, &projectRoot, &specJSON, &cacheRef); err != nil {
			rows.Close()
			return fmt.Errorf("scan environment backfill profile: %w", err)
		}
		var spec EnvironmentSpec
		if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
			rows.Close()
			return fmt.Errorf("decode environment backfill spec: %w", err)
		}
		values = append(values, backfillRow{
			workspaceID: workspaceID, repository: repository, projectRoot: projectRoot,
			spec: spec, cacheRef: cacheRef,
		})
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, value := range values {
		environment, err := s.FindOrCreateEnvironment(ctx, value.repository, value.projectRoot)
		if err != nil {
			return err
		}
		var version EnvironmentVersion
		version, found, err := s.FindReadyEnvironmentVersion(ctx, environment.ID, value.spec.Fingerprint)
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
