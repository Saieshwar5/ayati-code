package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (s *Store) SaveProfile(ctx context.Context, workspaceID string, profile ProjectProfile) error {
	languages, err := json.Marshal(nonNilStrings(profile.Languages))
	if err != nil {
		return err
	}
	runtimes, err := json.Marshal(nonNilStrings(profile.RuntimeVersions))
	if err != nil {
		return err
	}
	managers, err := json.Marshal(nonNilStrings(profile.PackageManagers))
	if err != nil {
		return err
	}
	lockfiles, err := json.Marshal(nonNilStrings(profile.Lockfiles))
	if err != nil {
		return err
	}
	preparedAt := ""
	if profile.PreparedAt != nil {
		preparedAt = formatTime(profile.PreparedAt.UTC())
	}
	environmentSpec := ""
	if profile.EnvironmentSpec != nil {
		encoded, err := json.Marshal(profile.EnvironmentSpec)
		if err != nil {
			return fmt.Errorf("encode environment spec: %w", err)
		}
		environmentSpec = string(encoded)
	}
	result, err := s.execContext(ctx, `INSERT INTO workspace_profiles (
		workspace_id, project_root, languages, runtime_versions, package_managers, lockfiles,
		setup_command, test_command, lint_command, typecheck_command, build_command,
		instructions_file, manifest_fingerprint, baseline_commit, setup_result,
		baseline_result, cache_path, environment_spec, prepared_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(workspace_id) DO UPDATE SET
		project_root = excluded.project_root, languages = excluded.languages,
		runtime_versions = excluded.runtime_versions, package_managers = excluded.package_managers,
		lockfiles = excluded.lockfiles, setup_command = excluded.setup_command,
		test_command = excluded.test_command, lint_command = excluded.lint_command,
		typecheck_command = excluded.typecheck_command, build_command = excluded.build_command,
		instructions_file = excluded.instructions_file,
		manifest_fingerprint = excluded.manifest_fingerprint,
		baseline_commit = excluded.baseline_commit, setup_result = excluded.setup_result,
		baseline_result = excluded.baseline_result, cache_path = excluded.cache_path,
		environment_spec = excluded.environment_spec, prepared_at = excluded.prepared_at`,
		strings.TrimSpace(workspaceID), profile.ProjectRoot, string(languages), string(runtimes),
		string(managers), string(lockfiles), profile.SetupCommand, profile.TestCommand,
		profile.LintCommand, profile.TypecheckCommand, profile.BuildCommand,
		profile.InstructionsFile, profile.ManifestFingerprint, profile.BaselineCommit,
		profile.SetupResult, profile.BaselineResult, profile.CachePath, environmentSpec,
		preparedAt,
	)
	if err != nil {
		return fmt.Errorf("save workspace profile: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ProjectProfile(ctx context.Context, workspaceID string) (*ProjectProfile, error) {
	row := s.queryRowContext(ctx, `SELECT project_root, languages, runtime_versions,
		package_managers, lockfiles, setup_command, test_command, lint_command,
		typecheck_command, build_command, instructions_file, manifest_fingerprint,
		baseline_commit, setup_result, baseline_result, cache_path, environment_spec,
		prepared_at
		FROM workspace_profiles WHERE workspace_id = ?`, strings.TrimSpace(workspaceID))
	var profile ProjectProfile
	var languages, runtimes, managers, lockfiles, environmentSpec, preparedAt string
	err := row.Scan(&profile.ProjectRoot, &languages, &runtimes, &managers, &lockfiles,
		&profile.SetupCommand, &profile.TestCommand, &profile.LintCommand,
		&profile.TypecheckCommand, &profile.BuildCommand, &profile.InstructionsFile,
		&profile.ManifestFingerprint, &profile.BaselineCommit, &profile.SetupResult,
		&profile.BaselineResult, &profile.CachePath, &environmentSpec, &preparedAt)
	if err != nil {
		return nil, err
	}
	for _, value := range []struct {
		encoded string
		target  *[]string
	}{
		{languages, &profile.Languages}, {runtimes, &profile.RuntimeVersions},
		{managers, &profile.PackageManagers}, {lockfiles, &profile.Lockfiles},
	} {
		if err := json.Unmarshal([]byte(value.encoded), value.target); err != nil {
			return nil, fmt.Errorf("decode workspace profile: %w", err)
		}
	}
	if environmentSpec != "" && environmentSpec != "{}" {
		var spec EnvironmentSpec
		if err := json.Unmarshal([]byte(environmentSpec), &spec); err != nil {
			return nil, fmt.Errorf("decode environment spec: %w", err)
		}
		profile.EnvironmentSpec = &spec
	}
	if preparedAt != "" {
		value, err := time.Parse(time.RFC3339Nano, preparedAt)
		if err != nil {
			return nil, fmt.Errorf("decode workspace preparation time: %w", err)
		}
		profile.PreparedAt = &value
	}
	return &profile, nil
}

func (s *Store) attachProfile(ctx context.Context, workspace *Workspace) error {
	profile, err := s.ProjectProfile(ctx, workspace.ID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	workspace.Profile = profile
	return nil
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
