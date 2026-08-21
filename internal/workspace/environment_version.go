package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	EnvironmentVersionPending = "pending"
	EnvironmentVersionReady   = "ready"
	EnvironmentVersionFailed  = "failed"

	SnapshotTypeLocalCopy = "local_copy"
)

// Environment is the stable identity for a repository project root. It stays
// the same as toolchain and dependency definitions evolve; those changes are
// recorded as new EnvironmentVersion rows instead.
type Environment struct {
	ID          string
	Repository  string
	ProjectRoot string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// EnvironmentVersion is one concrete, potentially reusable build definition
// for an Environment.
type EnvironmentVersion struct {
	ID                string
	EnvironmentID     string
	Version           int
	SourceFingerprint string
	Spec              EnvironmentSpec
	State             string
	ArtifactRef       string
	CacheRef          string
	Error             string
	SnapshotType      string
	SnapshotRef       string
	SnapshotManifest  []string
	SnapshotBytes     int64
	SnapshotCreatedAt time.Time
	ReadyAt           time.Time
	CreatedAt         time.Time
}

const projectEnvironmentsSchema = `CREATE TABLE IF NOT EXISTS project_environments (
	id TEXT PRIMARY KEY,
	repository TEXT NOT NULL,
	project_root TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE(repository, project_root)
)`

const environmentVersionsSchema = `CREATE TABLE IF NOT EXISTS environment_versions (
	id TEXT PRIMARY KEY,
	environment_id TEXT NOT NULL REFERENCES project_environments(id) ON DELETE CASCADE,
	version INTEGER NOT NULL,
	source_fingerprint TEXT NOT NULL,
	spec TEXT NOT NULL,
	state TEXT NOT NULL,
	artifact_ref TEXT NOT NULL DEFAULT '',
	cache_ref TEXT NOT NULL DEFAULT '',
	error TEXT NOT NULL DEFAULT '',
	snapshot_type TEXT NOT NULL DEFAULT '',
	snapshot_ref TEXT NOT NULL DEFAULT '',
	snapshot_manifest TEXT NOT NULL DEFAULT '',
	snapshot_bytes INTEGER NOT NULL DEFAULT 0,
	snapshot_created_at TEXT NOT NULL DEFAULT '',
	ready_at TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
)`

func (s *Store) FindOrCreateEnvironment(ctx context.Context, repository, projectRoot string) (Environment, error) {
	repository, projectRoot = strings.TrimSpace(repository), strings.TrimSpace(projectRoot)
	if repository == "" || projectRoot == "" {
		return Environment{}, errors.New("repository and project root are required")
	}
	var value Environment
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `SELECT id, repository, project_root,
		created_at, updated_at FROM project_environments
		WHERE repository = ? AND project_root = ?`, repository, projectRoot).
		Scan(&value.ID, &value.Repository, &value.ProjectRoot, &createdAt, &updatedAt)
	if err == nil {
		parsedCreated, parseErr := parseStoredTime(createdAt)
		if parseErr != nil {
			return Environment{}, parseErr
		}
		parsedUpdated, parseErr := parseStoredTime(updatedAt)
		if parseErr != nil {
			return Environment{}, parseErr
		}
		value.CreatedAt, value.UpdatedAt = parsedCreated, parsedUpdated
		return value, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Environment{}, fmt.Errorf("find environment: %w", err)
	}
	id, err := newID()
	if err != nil {
		return Environment{}, err
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `INSERT INTO project_environments (id, repository, project_root,
		created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		id, repository, projectRoot, formatTime(now), formatTime(now))
	if err != nil {
		return Environment{}, fmt.Errorf("create environment: %w", err)
	}
	return Environment{
		ID: id, Repository: repository, ProjectRoot: projectRoot,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (s *Store) FindReadyEnvironmentVersion(
	ctx context.Context, environmentID, fingerprint string,
) (EnvironmentVersion, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, environment_id, version, source_fingerprint,
		spec, state, artifact_ref, cache_ref, error, snapshot_type, snapshot_ref,
		snapshot_manifest, snapshot_bytes, snapshot_created_at, ready_at, created_at
		FROM environment_versions WHERE environment_id = ? AND source_fingerprint = ?
		AND state = ? ORDER BY version DESC LIMIT 1`,
		strings.TrimSpace(environmentID), strings.TrimSpace(fingerprint), EnvironmentVersionReady)
	value, err := scanEnvironmentVersion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return EnvironmentVersion{}, false, nil
	}
	if err != nil {
		return EnvironmentVersion{}, false, err
	}
	return value, true, nil
}

func (s *Store) CreateEnvironmentVersion(
	ctx context.Context, environmentID, fingerprint string, spec EnvironmentSpec, cacheRef string,
) (EnvironmentVersion, error) {
	environmentID, fingerprint = strings.TrimSpace(environmentID), strings.TrimSpace(fingerprint)
	if environmentID == "" || fingerprint == "" {
		return EnvironmentVersion{}, errors.New("environment ID and fingerprint are required")
	}
	encoded, err := json.Marshal(spec)
	if err != nil {
		return EnvironmentVersion{}, fmt.Errorf("encode environment version spec: %w", err)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM environment_versions
		WHERE environment_id = ?`, environmentID).Scan(&count); err != nil {
		return EnvironmentVersion{}, fmt.Errorf("count environment versions: %w", err)
	}
	id, err := newID()
	if err != nil {
		return EnvironmentVersion{}, err
	}
	now := time.Now().UTC()
	value := EnvironmentVersion{
		ID: id, EnvironmentID: environmentID, Version: count + 1,
		SourceFingerprint: fingerprint, Spec: spec, State: EnvironmentVersionPending,
		CacheRef: strings.TrimSpace(cacheRef), CreatedAt: now,
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO environment_versions (
		id, environment_id, version, source_fingerprint, spec, state, artifact_ref,
		cache_ref, created_at
	) VALUES (?, ?, ?, ?, ?, ?, '', ?, ?)`, value.ID, value.EnvironmentID,
		value.Version, value.SourceFingerprint, string(encoded), value.State,
		value.CacheRef, formatTime(value.CreatedAt))
	if err != nil {
		return EnvironmentVersion{}, fmt.Errorf("create environment version: %w", err)
	}
	return value, nil
}

func (s *Store) BindWorkspaceEnvironment(ctx context.Context, workspaceID, versionID string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE workspaces SET environment_version_id = ?
		WHERE id = ?`, strings.TrimSpace(versionID), strings.TrimSpace(workspaceID))
	if err != nil {
		return fmt.Errorf("bind workspace environment: %w", err)
	}
	return requireOneRow(result)
}

func (s *Store) SetEnvironmentVersionState(ctx context.Context, id, state, message string) error {
	if state != EnvironmentVersionPending && state != EnvironmentVersionReady && state != EnvironmentVersionFailed {
		return fmt.Errorf("invalid environment version state %q", state)
	}
	readyAt := ""
	if state == EnvironmentVersionReady {
		readyAt = formatTime(time.Now().UTC())
	}
	result, err := s.db.ExecContext(ctx, `UPDATE environment_versions SET state = ?,
		error = ?, ready_at = ? WHERE id = ?`, state, strings.TrimSpace(message),
		readyAt, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("set environment version state: %w", err)
	}
	return requireOneRow(result)
}

func (s *Store) SetEnvironmentVersionSnapshot(
	ctx context.Context, id, snapshotType, snapshotRef string, manifest []string, snapshotBytes int64,
) error {
	encoded, err := json.Marshal(nonNilStrings(manifest))
	if err != nil {
		return fmt.Errorf("encode environment snapshot manifest: %w", err)
	}
	now := formatTime(time.Now().UTC())
	result, err := s.db.ExecContext(ctx, `UPDATE environment_versions SET
		snapshot_type = ?, snapshot_ref = ?, snapshot_manifest = ?, snapshot_bytes = ?,
		snapshot_created_at = ? WHERE id = ?`,
		strings.TrimSpace(snapshotType), strings.TrimSpace(snapshotRef), string(encoded),
		snapshotBytes, now, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("set environment version snapshot: %w", err)
	}
	return requireOneRow(result)
}

func (s *Store) GetEnvironmentVersion(ctx context.Context, id string) (EnvironmentVersion, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, environment_id, version, source_fingerprint,
		spec, state, artifact_ref, cache_ref, error, snapshot_type, snapshot_ref,
		snapshot_manifest, snapshot_bytes, snapshot_created_at, ready_at, created_at
		FROM environment_versions WHERE id = ?`, strings.TrimSpace(id))
	return scanEnvironmentVersion(row)
}

func scanEnvironmentVersion(row scanner) (EnvironmentVersion, error) {
	var value EnvironmentVersion
	var specJSON, manifestJSON, snapshotCreatedAt, readyAt, createdAt string
	err := row.Scan(&value.ID, &value.EnvironmentID, &value.Version, &value.SourceFingerprint,
		&specJSON, &value.State, &value.ArtifactRef, &value.CacheRef, &value.Error,
		&value.SnapshotType, &value.SnapshotRef, &manifestJSON, &value.SnapshotBytes,
		&snapshotCreatedAt, &readyAt, &createdAt)
	if err != nil {
		return EnvironmentVersion{}, err
	}
	if err := json.Unmarshal([]byte(specJSON), &value.Spec); err != nil {
		return EnvironmentVersion{}, fmt.Errorf("decode environment version spec: %w", err)
	}
	if manifestJSON != "" {
		if err := json.Unmarshal([]byte(manifestJSON), &value.SnapshotManifest); err != nil {
			return EnvironmentVersion{}, fmt.Errorf("decode environment version snapshot manifest: %w", err)
		}
	}
	for _, pair := range []struct {
		encoded string
		target  *time.Time
	}{
		{snapshotCreatedAt, &value.SnapshotCreatedAt},
		{readyAt, &value.ReadyAt},
		{createdAt, &value.CreatedAt},
	} {
		if pair.encoded == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, pair.encoded)
		if err != nil {
			return EnvironmentVersion{}, fmt.Errorf("decode environment version time: %w", err)
		}
		*pair.target = parsed
	}
	return value, nil
}
