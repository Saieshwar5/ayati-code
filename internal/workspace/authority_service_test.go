package workspace

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/agent"
)

func TestServiceSealsExploreWorkspaceAfterSetup(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	path := filepath.Join(t.TempDir(), "repo")
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Setup: "npm ci", Path: path,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	environment := &fakeEnvironment{shell: &recordingShell{result: agent.ShellResult{ExitCode: 0}}}
	service := &Service{store: store, environment: environment, git: &recordingGit{}}
	if err := service.Initialize(context.Background(), value.ID); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if len(environment.ensured) != 2 || !environment.ensured[0].WorkspaceWritable ||
		environment.ensured[1].WorkspaceWritable || len(environment.removed) != 1 {
		t.Fatalf("sandbox lifecycle = ensured %#v, removed %#v", environment.ensured, environment.removed)
	}
	loaded, err := store.Get(context.Background(), value.ID)
	if err != nil || loaded.Authority != AuthorityExplore || loaded.EffectiveMountMode != "ro" {
		t.Fatalf("workspace = %#v, error = %v", loaded, err)
	}
}

func TestServiceRejectsPublishingFromExplore(t *testing.T) {
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
	service := &Service{store: store, environment: &fakeEnvironment{}, git: &recordingGit{}}
	err = service.Publish(context.Background(), value.ID, "feat: change", "Perpetual", "ayati@example.test")
	if err == nil || err.Error() != "publishing requires Develop authority" {
		t.Fatalf("Publish error = %v", err)
	}
}
