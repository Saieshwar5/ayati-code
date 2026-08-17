package workspace

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceArchiveStopsSandboxAndBlocksWorkspaceUse(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
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
	if err := store.UpdateStatus(context.Background(), value.ID, StatusReady, ""); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	environment := &fakeEnvironment{}
	service := &Service{store: store, environment: environment}

	if err := service.Archive(context.Background(), value.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if len(environment.removed) != 1 || environment.removed[0] != value.SandboxName {
		t.Fatalf("removed sandboxes = %#v", environment.removed)
	}
	loaded, err := store.Get(context.Background(), value.ID)
	if err != nil || loaded.Status != StatusStopped || loaded.ArchivedAt == nil {
		t.Fatalf("archived workspace = %#v, error = %v", loaded, err)
	}
	if _, _, err := service.Shell(context.Background(), value.ID); err == nil ||
		!strings.Contains(err.Error(), "archived") {
		t.Fatalf("Shell error = %v", err)
	}
	if _, err := store.CreateSession(context.Background(), value.ID, "Blocked"); err == nil ||
		!strings.Contains(err.Error(), "archived") {
		t.Fatalf("CreateSession error = %v", err)
	}
}
