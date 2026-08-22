package workspace

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigrationRemovesComputeEnvironmentSandboxSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "perpetual.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Simulate the sandbox-era schema: the legacy column and compute tables.
	if _, err := store.db.Exec(`ALTER TABLE workspaces ADD COLUMN sandbox_name TEXT NOT NULL DEFAULT ''`); err != nil {
		t.Fatalf("prepare legacy sandbox column: %v", err)
	}
	for _, statement := range []string{
		`CREATE TABLE environments (id TEXT PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE environment_leases (id TEXT PRIMARY KEY, environment_id TEXT NOT NULL)`,
		`CREATE TRIGGER environments_prevent_active_delete BEFORE DELETE ON environments
			BEGIN SELECT RAISE(ABORT, 'environment is occupied'); END`,
		`CREATE TRIGGER workspaces_prevent_active_lease_delete BEFORE DELETE ON workspaces
			BEGIN SELECT RAISE(ABORT, 'workspace has an active environment lease'); END`,
	} {
		if _, err := store.db.Exec(statement); err != nil {
			t.Fatalf("prepare legacy schema: %v", err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatalf("reopen migrated database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	loaded, err := store.Get(context.Background(), value.ID)
	if err != nil || loaded.Repository != "owner/project" || loaded.Branch != "main" {
		t.Fatalf("workspace = %#v, error = %v", loaded, err)
	}
	for _, table := range []string{"environments", "environment_leases"} {
		var count int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master
			WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("table %s still present: %#v, error = %v", table, count, err)
		}
	}
	var triggers int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'trigger' AND name IN ('environments_prevent_active_delete',
		'workspaces_prevent_active_lease_delete')`).Scan(&triggers); err != nil || triggers != 0 {
		t.Fatalf("legacy triggers still present: %#v, error = %v", triggers, err)
	}
	columns, err := databaseColumns(context.Background(), store.db, store.database.Dialect(), "workspaces")
	if err != nil {
		t.Fatalf("databaseColumns: %v", err)
	}
	if columns["sandbox_name"] {
		t.Fatalf("sandbox_name column remains: %#v", columns)
	}
	sessions, err := store.ListSessions(context.Background(), value.ID)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("preserved sessions = %#v, error = %v", sessions, err)
	}
}
