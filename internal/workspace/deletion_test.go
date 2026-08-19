package workspace

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestServiceDeletesWorkspaceWithReadOnlyModuleCache(t *testing.T) {
	root, store, value := deletionWorkspace(t)
	module := filepath.Join(root, value.ID, "cache", "go-mod", "example.com", "module@v1.0.0")
	if err := os.MkdirAll(module, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(module, "README.md"), []byte("cached"), 0o444); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chmod(module, 0o555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	environment := &fakeEnvironment{}
	git := &recordingGit{}
	service := &Service{store: store, environment: environment, git: git, root: root}

	if err := service.Delete(context.Background(), value.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, value.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace directory still exists: %v", err)
	}
	if _, err := store.Get(context.Background(), value.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("workspace record still exists: %v", err)
	}
	if len(environment.removed) != 1 || len(git.calls) != 0 {
		t.Fatalf("environment cleanup = %#v, Git calls = %#v", environment.removed, git.calls)
	}
}

func TestServiceRetriesDeletionWithoutFollowingWorkspaceSymlink(t *testing.T) {
	root, store, value := deletionWorkspace(t)
	outside := t.TempDir()
	marker := filepath.Join(outside, "preserved.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	workspaceDirectory := filepath.Join(root, value.ID)
	if err := os.Symlink(outside, workspaceDirectory); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	service := &Service{store: store, environment: &fakeEnvironment{}, git: &recordingGit{}, root: root}

	err := service.Delete(context.Background(), value.ID)
	if err == nil {
		t.Fatal("Delete unexpectedly followed workspace symlink")
	}
	loaded, loadErr := store.Get(context.Background(), value.ID)
	if loadErr != nil || loaded.Status != StatusDeletionFailed {
		t.Fatalf("workspace = %#v, error = %v", loaded, loadErr)
	}
	if data, readErr := os.ReadFile(marker); readErr != nil || string(data) != "keep" {
		t.Fatalf("outside marker = %q, error = %v", data, readErr)
	}
	if err := os.Remove(workspaceDirectory); err != nil {
		t.Fatalf("Remove symlink: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspaceDirectory, "repo"), 0o700); err != nil {
		t.Fatalf("MkdirAll retry target: %v", err)
	}
	if err := service.Delete(context.Background(), value.ID); err != nil {
		t.Fatalf("retry Delete: %v", err)
	}
}

func TestServiceSerializesDuplicateWorkspaceDeletion(t *testing.T) {
	root, store, value := deletionWorkspace(t)
	if err := os.MkdirAll(value.Path, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	environment := &fakeEnvironment{}
	service := &Service{store: store, environment: environment, git: &recordingGit{}, root: root}
	errorsFound := make(chan error, 2)
	var requests sync.WaitGroup
	for range 2 {
		requests.Add(1)
		go func() {
			defer requests.Done()
			errorsFound <- service.Delete(context.Background(), value.ID)
		}()
	}
	requests.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatalf("duplicate Delete: %v", err)
		}
	}
	if len(environment.removed) != 1 {
		t.Fatalf("environment cleanup = %#v", environment.removed)
	}
}

func TestServiceRecoveryFinishesArchivedWorkspaceDeletion(t *testing.T) {
	root, store, value := deletionWorkspace(t)
	if err := os.MkdirAll(value.Path, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := store.UpdateStatus(context.Background(), value.ID, StatusDeletionFailed, "retry on restart"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if err := store.Archive(context.Background(), value.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	service := &Service{store: store, environment: &fakeEnvironment{}, git: &recordingGit{}, root: root}

	if err := service.Recover(context.Background()); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if _, err := store.Get(context.Background(), value.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("workspace record still exists: %v", err)
	}
}

func deletionWorkspace(t *testing.T) (string, *Store, Workspace) {
	t.Helper()
	root := t.TempDir()
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "perpetual/delete", Root: root,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.UpdateStatus(context.Background(), value.ID, StatusReady, ""); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	value.Status = StatusReady
	return root, store, value
}
