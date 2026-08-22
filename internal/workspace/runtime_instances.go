package workspace

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// RuntimeInstance is the durable provider-side instance record for a workspace
// runtime (for example a Lambda MicroVM id and endpoint). It survives
// controller restarts so providers can suspend, resume, terminate, and
// reconcile.
type RuntimeInstance struct {
	WorkspaceID  string
	Provider     string
	InstanceID   string
	Endpoint     string
	ImageARN     string
	ImageVersion string
	State        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

const runtimeInstancesSchema = `CREATE TABLE IF NOT EXISTS runtime_instances (
	workspace_id TEXT PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
	provider TEXT NOT NULL,
	instance_id TEXT NOT NULL,
	endpoint TEXT NOT NULL DEFAULT '',
	image_arn TEXT NOT NULL DEFAULT '',
	image_version TEXT NOT NULL DEFAULT '',
	state TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
)`

func (s *Store) createRuntimeInstanceTable() error {
	for _, statement := range []string{
		runtimeInstancesSchema,
		`CREATE INDEX IF NOT EXISTS runtime_instances_provider ON runtime_instances(provider, state)`,
	} {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

// SaveRuntimeInstance upserts the runtime instance for a workspace.
func (s *Store) SaveRuntimeInstance(ctx context.Context, instance RuntimeInstance) error {
	instance.WorkspaceID = strings.TrimSpace(instance.WorkspaceID)
	instance.Provider = strings.TrimSpace(instance.Provider)
	instance.InstanceID = strings.TrimSpace(instance.InstanceID)
	if instance.WorkspaceID == "" || instance.Provider == "" || instance.InstanceID == "" {
		return errors.New("workspace ID, provider, and instance ID are required")
	}
	now := formatTime(time.Now().UTC())
	_, err := s.execContext(ctx, `INSERT INTO runtime_instances (
		workspace_id, provider, instance_id, endpoint, image_arn, image_version,
		state, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(workspace_id) DO UPDATE SET
		provider = excluded.provider,
		instance_id = excluded.instance_id,
		endpoint = excluded.endpoint,
		image_arn = excluded.image_arn,
		image_version = excluded.image_version,
		state = excluded.state,
		updated_at = excluded.updated_at`,
		instance.WorkspaceID, instance.Provider, instance.InstanceID,
		strings.TrimSpace(instance.Endpoint), strings.TrimSpace(instance.ImageARN),
		strings.TrimSpace(instance.ImageVersion), strings.TrimSpace(instance.State),
		now, now)
	if err != nil {
		return err
	}
	return nil
}

// RuntimeInstance returns the persisted instance for a workspace.
func (s *Store) RuntimeInstance(ctx context.Context, workspaceID string) (RuntimeInstance, error) {
	row := s.queryRowContext(ctx, `SELECT workspace_id, provider, instance_id, endpoint,
		image_arn, image_version, state, created_at, updated_at
		FROM runtime_instances WHERE workspace_id = ?`, strings.TrimSpace(workspaceID))
	return scanRuntimeInstance(row)
}

func (s *Store) ListRuntimeInstances(ctx context.Context) ([]RuntimeInstance, error) {
	rows, err := s.queryContext(ctx, `SELECT workspace_id, provider, instance_id, endpoint,
		image_arn, image_version, state, created_at, updated_at
		FROM runtime_instances ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []RuntimeInstance
	for rows.Next() {
		value, err := scanRuntimeInstanceRow(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

// DeleteRuntimeInstance removes the instance record (used after termination).
func (s *Store) DeleteRuntimeInstance(ctx context.Context, workspaceID string) error {
	_, err := s.execContext(ctx, `DELETE FROM runtime_instances WHERE workspace_id = ?`,
		strings.TrimSpace(workspaceID))
	return err
}

func scanRuntimeInstance(row *sql.Row) (RuntimeInstance, error) {
	return scanRuntimeInstanceRow(row)
}

func scanRuntimeInstanceRow(row scanner) (RuntimeInstance, error) {
	var value RuntimeInstance
	var createdAt, updatedAt string
	err := row.Scan(&value.WorkspaceID, &value.Provider, &value.InstanceID,
		&value.Endpoint, &value.ImageARN, &value.ImageVersion, &value.State,
		&createdAt, &updatedAt)
	if err != nil {
		return RuntimeInstance{}, err
	}
	value.CreatedAt, err = parseStoredTime(createdAt)
	if err != nil {
		return RuntimeInstance{}, err
	}
	value.UpdatedAt, err = parseStoredTime(updatedAt)
	if err != nil {
		return RuntimeInstance{}, err
	}
	return value, nil
}
