package environment

import (
	"fmt"
)

const environmentSchema = `CREATE TABLE IF NOT EXISTS environments (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	driver TEXT NOT NULL,
	image_ref TEXT NOT NULL,
	image_digest TEXT NOT NULL DEFAULT '',
	cpu_millis INTEGER NOT NULL,
	memory_mb INTEGER NOT NULL,
	pid_limit INTEGER NOT NULL,
	network_policy TEXT NOT NULL CHECK (network_policy IN ('disabled', 'outbound')),
	provisioning_state TEXT NOT NULL CHECK (provisioning_state IN ('provisioning', 'ready', 'failed', 'deleting')),
	generation INTEGER NOT NULL DEFAULT 0,
	error TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
)`

const leaseSchema = `CREATE TABLE IF NOT EXISTS environment_leases (
	id TEXT PRIMARY KEY,
	environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
	workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	generation INTEGER NOT NULL,
	state TEXT NOT NULL CHECK (state IN ('acquiring', 'active', 'releasing', 'released', 'failed')),
	runtime_id TEXT NOT NULL DEFAULT '',
	error TEXT NOT NULL DEFAULT '',
	acquired_at TEXT NOT NULL,
	activated_at TEXT NOT NULL DEFAULT '',
	released_at TEXT NOT NULL DEFAULT ''
)`

func (s *Store) configure() error {
	statements := []string{
		environmentSchema,
		leaseSchema,
		`CREATE UNIQUE INDEX IF NOT EXISTS environment_leases_environment_active
			ON environment_leases(environment_id) WHERE state IN ('acquiring', 'active', 'releasing')`,
		`CREATE UNIQUE INDEX IF NOT EXISTS environment_leases_workspace_active
			ON environment_leases(workspace_id) WHERE state IN ('acquiring', 'active', 'releasing')`,
		`CREATE INDEX IF NOT EXISTS environment_leases_workspace_history
			ON environment_leases(workspace_id, acquired_at DESC)`,
		`CREATE TRIGGER IF NOT EXISTS environments_prevent_active_delete
			BEFORE DELETE ON environments
			WHEN EXISTS (SELECT 1 FROM environment_leases WHERE environment_id = OLD.id
				AND state IN ('acquiring', 'active', 'releasing'))
			BEGIN SELECT RAISE(ABORT, 'environment is occupied'); END`,
		`CREATE TRIGGER IF NOT EXISTS workspaces_prevent_active_lease_delete
			BEFORE DELETE ON workspaces
			WHEN EXISTS (SELECT 1 FROM environment_leases WHERE workspace_id = OLD.id
				AND state IN ('acquiring', 'active', 'releasing'))
			BEGIN SELECT RAISE(ABORT, 'workspace has an active environment lease'); END`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("initialize environments: %w", err)
		}
	}
	return nil
}
