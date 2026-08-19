package workspace

import (
	"context"
	"path/filepath"
	"testing"
)

func TestWorkspaceArchivePreservesAndRestoresRecord(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.Archive(context.Background(), value.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	active, err := store.List(context.Background())
	if err != nil || len(active) != 0 {
		t.Fatalf("active workspaces = %#v, error = %v", active, err)
	}
	archived, err := store.ListArchived(context.Background())
	if err != nil || len(archived) != 1 || archived[0].ArchivedAt == nil {
		t.Fatalf("archived workspaces = %#v, error = %v", archived, err)
	}
	if sessions, err := store.ListSessions(context.Background(), value.ID); err != nil || len(sessions) != 1 {
		t.Fatalf("preserved sessions = %#v, error = %v", sessions, err)
	}

	if err := store.Restore(context.Background(), value.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	active, err = store.List(context.Background())
	if err != nil || len(active) != 1 || active[0].ArchivedAt != nil {
		t.Fatalf("restored workspaces = %#v, error = %v", active, err)
	}
}
