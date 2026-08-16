package workspace

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

func TestExplorePreparationRejectsProjectChanges(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Setup: "npm install", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	environment := &fakeEnvironment{shell: &recordingShell{result: agent.ShellResult{ExitCode: 0}}}
	git := &recordingGit{statusResults: []string{"", "?? package-lock.json\n"}}
	service := &Service{store: store, environment: environment, git: git}
	err = service.Initialize(context.Background(), value.ID)
	if err == nil || !strings.Contains(err.Error(), "setup modified project files") {
		t.Fatalf("Initialize error = %v", err)
	}
	loaded, loadErr := store.Get(context.Background(), value.ID)
	if loadErr != nil || loaded.Status != StatusInitializationFailed || loaded.Profile == nil ||
		loaded.Profile.SetupResult != "passed" || loaded.Profile.BaselineResult != "changed" {
		t.Fatalf("workspace = %#v, error = %v", loaded, loadErr)
	}
	if len(environment.ensured) != 1 || len(environment.removed) != 1 {
		t.Fatalf("sandbox lifecycle = ensured %#v, removed %#v", environment.ensured, environment.removed)
	}
}

func TestDevelopPreparationRecordsAllowedProjectChanges(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "ayati/change", Authority: AuthorityDevelop,
		Setup: "npm install", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	environment := &fakeEnvironment{shell: &recordingShell{result: agent.ShellResult{ExitCode: 0}}}
	service := &Service{store: store, environment: environment,
		git: &recordingGit{statusResults: []string{"", " M package-lock.json\n"}}}
	if err := service.Initialize(context.Background(), value.ID); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	loaded, _ := store.Get(context.Background(), value.ID)
	if loaded.Status != StatusReady || loaded.Profile == nil || loaded.Profile.BaselineResult != "changed" {
		t.Fatalf("workspace = %#v", loaded)
	}
}
