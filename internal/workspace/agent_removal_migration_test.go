package workspace

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestMigrationRemovesAgentRunsAndPreservesSessions(t *testing.T) {
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
	sessions, err := store.ListSessions(context.Background(), value.ID)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %#v, error = %v", sessions, err)
	}
	// Simulate the agent-era schema with an active run and a stored message.
	now := formatTime(time.Now().UTC())
	if _, err := store.db.Exec(`CREATE TABLE agent_runs (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
		session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		input TEXT NOT NULL,
		status TEXT NOT NULL,
		error TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		started_at TEXT NOT NULL DEFAULT '',
		finished_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("prepare legacy schema: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO agent_runs
		(id, workspace_id, session_id, input, status, created_at, updated_at)
		VALUES ('run-1', ?, ?, 'agent work', 'running', ?, ?)`,
		value.ID, sessions[0].ID, now, now); err != nil {
		t.Fatalf("insert legacy run: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatalf("reopen migrated database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name = 'agent_runs'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("agent_runs table still present: %#v, error = %v", count, err)
	}
	loaded, err := store.Get(context.Background(), value.ID)
	if err != nil || loaded.Repository != "owner/project" {
		t.Fatalf("workspace = %#v, error = %v", loaded, err)
	}
	remaining, err := store.ListSessions(context.Background(), value.ID)
	if err != nil || len(remaining) != 1 || remaining[0].ID != sessions[0].ID {
		t.Fatalf("preserved sessions = %#v, error = %v", remaining, err)
	}
}
