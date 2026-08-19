package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/agent"
)

func TestExplorePreparationRejectsProjectChanges(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
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
		loaded.Profile.SetupResult != "passed" || loaded.Profile.BaselineResult != "changed" ||
		loaded.PreparationStage != PreparationFailed || loaded.PreparationFailedStage != PreparationVerifying {
		t.Fatalf("workspace = %#v, error = %v", loaded, loadErr)
	}
	if len(environment.ensured) != 1 || len(environment.removed) != 1 {
		t.Fatalf("sandbox lifecycle = ensured %#v, removed %#v", environment.ensured, environment.removed)
	}
}

func TestDevelopPreparationRecordsAllowedProjectChanges(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "perpetual/change", Authority: AuthorityDevelop,
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
	if loaded.Status != StatusReady || loaded.PreparationStage != PreparationReady ||
		loaded.Profile == nil || loaded.Profile.BaselineResult != "changed" {
		t.Fatalf("workspace = %#v", loaded)
	}
}

func TestPreparationPausesForProjectSelectionAndContinues(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repository := filepath.Join(t.TempDir(), "repo")
	for _, root := range []string{"apps/api", "apps/web"} {
		path := filepath.Join(repository, filepath.FromSlash(root))
		if err := os.MkdirAll(filepath.Join(repository, ".git"), 0o700); err != nil {
			t.Fatalf("MkdirAll .git: %v", err)
		}
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatalf("MkdirAll project: %v", err)
		}
		if err := os.WriteFile(filepath.Join(path, "package.json"), []byte(`{}`), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Path: repository,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	service := &Service{
		store: store,
		environment: &fakeEnvironment{shell: &recordingShell{
			result: agent.ShellResult{ExitCode: 0},
		}},
		git: &recordingGit{},
	}
	err = service.Initialize(context.Background(), value.ID)
	var selection ProjectSelectionRequiredError
	if !errors.As(err, &selection) {
		t.Fatalf("Initialize error = %v", err)
	}
	loaded, _ := store.Get(context.Background(), value.ID)
	if loaded.Status != StatusNeedsConfiguration ||
		loaded.PreparationStage != PreparationNeedsConfiguration ||
		len(loaded.ConfigurationCandidates) != 2 {
		t.Fatalf("workspace = %#v", loaded)
	}
	if err := service.ConfigureProjectRoot(context.Background(), value.ID, "../outside"); err == nil {
		t.Fatal("ConfigureProjectRoot accepted an unavailable root")
	}
	if err := service.ConfigureProjectRoot(context.Background(), value.ID, "apps/web"); err != nil {
		t.Fatalf("ConfigureProjectRoot: %v", err)
	}
	if err := service.Initialize(context.Background(), value.ID); err != nil {
		t.Fatalf("Initialize configured workspace: %v", err)
	}
	loaded, _ = store.Get(context.Background(), value.ID)
	if loaded.Status != StatusReady || loaded.PreparationStage != PreparationReady ||
		loaded.SelectedProjectRoot != "apps/web" || loaded.Profile == nil ||
		loaded.Profile.ProjectRoot != "apps/web" || len(loaded.ConfigurationCandidates) != 0 {
		t.Fatalf("configured workspace = %#v", loaded)
	}
}
