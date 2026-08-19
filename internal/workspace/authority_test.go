package workspace

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestStoreMigratesExistingWorkspaceToDevelop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("Open legacy database: %v", err)
	}
	legacySchema := `CREATE TABLE workspaces (
		id TEXT PRIMARY KEY, repository TEXT NOT NULL, clone_url TEXT NOT NULL,
		base_branch TEXT NOT NULL, branch TEXT NOT NULL, create_branch INTEGER NOT NULL,
		setup_command TEXT NOT NULL, path TEXT NOT NULL UNIQUE, sandbox_name TEXT NOT NULL UNIQUE,
		status TEXT NOT NULL, error TEXT NOT NULL DEFAULT '', pull_request_number INTEGER NOT NULL DEFAULT 0,
		pull_request_url TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL
	)`
	if _, err := database.Exec(legacySchema); err != nil {
		t.Fatalf("Create legacy schema: %v", err)
	}
	now := formatTime(time.Now().UTC())
	if _, err := database.Exec(`INSERT INTO workspaces (
		id, repository, clone_url, base_branch, branch, create_branch, setup_command, path,
		sandbox_name, status, created_at, updated_at
	) VALUES ('legacy', 'owner/project', 'https://github.com/owner/project.git', 'main',
		'perpetual/change', 1, '', '/tmp/legacy-authority', 'perpetual-workspace-legacy', 'ready', ?, ?)`, now, now); err != nil {
		t.Fatalf("Insert legacy workspace: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close legacy database: %v", err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Migrate database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Get(context.Background(), "legacy")
	if err != nil || value.Authority != AuthorityDevelop || value.EffectiveMountMode != "" {
		t.Fatalf("workspace = %#v, error = %v", value, err)
	}
}

func TestParseAuthorityDefaultsToExplore(t *testing.T) {
	authority, err := ParseAuthority("")
	if err != nil || authority != AuthorityExplore {
		t.Fatalf("authority = %q, error = %v", authority, err)
	}
	if _, err := ParseAuthority("admin"); err == nil {
		t.Fatal("ParseAuthority accepted unknown authority")
	}
}
