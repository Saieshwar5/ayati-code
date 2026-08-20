package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreRecoversWorkInterruptedByRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "perpetual.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	interrupted := createTestWorkspace(t, store)
	ready := createTestWorkspace(t, store)
	if err := store.UpdateStatus(context.Background(), interrupted.ID, StatusInitializing, ""); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if err := store.UpdatePreparation(context.Background(), interrupted.ID,
		PreparationInstalling, "Installing dependencies"); err != nil {
		t.Fatalf("UpdatePreparation: %v", err)
	}
	sessions, err := store.ListSessions(context.Background(), interrupted.ID)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if err := store.UpdateSessionStatus(context.Background(), sessions[0].ID,
		SessionStatusWorking, ""); err != nil {
		t.Fatalf("UpdateSessionStatus: %v", err)
	}
	if err := store.CompletePreparation(context.Background(), ready.ID); err != nil {
		t.Fatalf("CompletePreparation: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	loaded, err := store.Get(context.Background(), interrupted.ID)
	if err != nil || loaded.Status != StatusInitializationFailed ||
		loaded.PreparationStage != PreparationFailed ||
		loaded.PreparationFailedStage != PreparationInstalling ||
		!strings.Contains(loaded.Error, "interrupted") {
		t.Fatalf("recovered workspace = %#v, error = %v", loaded, err)
	}
	sessions, err = store.ListSessions(context.Background(), interrupted.ID)
	if err != nil || sessions[0].Status != SessionStatusFailed ||
		!strings.Contains(sessions[0].Error, "interrupted") {
		t.Fatalf("recovered sessions = %#v, error = %v", sessions, err)
	}
	loadedReady, err := store.Get(context.Background(), ready.ID)
	if err != nil || loadedReady.Status != StatusReady || loadedReady.PreparationStage != PreparationReady {
		t.Fatalf("ready workspace = %#v, error = %v", loadedReady, err)
	}
}

func TestServiceResumesStoppedWorkspaceWithoutReinitializing(t *testing.T) {
	store, value := readyWorkspace(t, "perpetual/change", true)
	if err := os.MkdirAll(value.Path, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(value.Path, "uncommitted.go"), []byte("package change\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	git := &recordingGit{}
	service := &Service{store: store, git: git}
	if err := service.Stop(context.Background(), value.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := service.Resume(context.Background(), value.ID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	loaded, err := store.Get(context.Background(), value.ID)
	if err != nil || loaded.Status != StatusReady {
		t.Fatalf("workspace = %#v, error = %v", loaded, err)
	}
	if len(git.calls) != 0 {
		t.Fatalf("git = %#v", git.calls)
	}
	data, err := os.ReadFile(filepath.Join(value.Path, "uncommitted.go"))
	if err != nil || string(data) != "package change\n" {
		t.Fatalf("preserved change = %q, error = %v", data, err)
	}
}

func TestServiceRejectsInitializationOutsideCreationOrRetry(t *testing.T) {
	store, value := readyWorkspace(t, "main", false)
	service := &Service{store: store, git: &recordingGit{}}
	if err := service.Initialize(context.Background(), value.ID); err == nil ||
		!strings.Contains(err.Error(), "cannot be initialized") {
		t.Fatalf("Initialize error = %v", err)
	}
	if err := service.Resume(context.Background(), value.ID); err == nil ||
		!strings.Contains(err.Error(), "not stopped") {
		t.Fatalf("Resume error = %v", err)
	}
	if err := store.UpdateStatus(context.Background(), value.ID, StatusInitializationFailed, "failed"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if err := service.Stop(context.Background(), value.ID); err == nil ||
		!strings.Contains(err.Error(), "not ready") {
		t.Fatalf("Stop error = %v", err)
	}
}

func readyWorkspace(t *testing.T, branch string, createBranch bool) (*Store, Workspace) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: branch, CreateBranch: createBranch,
		Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.CompletePreparation(context.Background(), value.ID); err != nil {
		t.Fatalf("CompletePreparation: %v", err)
	}
	return store, value
}
