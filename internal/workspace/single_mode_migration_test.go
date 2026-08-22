package workspace

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSingleWorkspaceModeMigrationPreservesWorkspaceConversation(t *testing.T) {
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
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if err := store.AppendMessage(context.Background(), sessions[0].ID,
		Message{Role: "user", Content: "preserve this"}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	for _, statement := range []string{
		`ALTER TABLE workspaces ADD COLUMN authority TEXT NOT NULL DEFAULT 'explore' CHECK (authority IN ('explore', 'develop'))`,
		`ALTER TABLE workspaces ADD COLUMN effective_mount_mode TEXT NOT NULL DEFAULT 'ro'`,
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
	if err != nil || loaded.Branch != "main" || loaded.Repository != "owner/project" {
		t.Fatalf("workspace = %#v, error = %v", loaded, err)
	}
	messages, err := store.Messages(context.Background(), sessions[0].ID)
	if err != nil || len(messages) != 1 || messages[0].Content != "preserve this" {
		t.Fatalf("messages = %#v, error = %v", messages, err)
	}
	columns, err := databaseColumns(context.Background(), store.db, store.database.Dialect(), "workspaces")
	if err != nil {
		t.Fatalf("databaseColumns: %v", err)
	}
	if columns["authority"] || columns["effective_mount_mode"] {
		t.Fatalf("legacy columns remain: %#v", columns)
	}
}
