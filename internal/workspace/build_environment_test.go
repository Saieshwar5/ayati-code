package workspace

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/exec"
)

func TestStartEnvironmentBuildEnqueuesAndBuildsVersion(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, version := boundEnvironmentBuildWorkspace(t, store, EnvironmentVersionPending)
	service := &Service{
		store:   store,
		runtime: &fakeRuntime{shell: &recordingShell{result: exec.ShellResult{ExitCode: 0}}},
		git:     &recordingGit{},
	}
	if err := service.StartEnvironmentBuild(context.Background(), value.ID); err != nil {
		t.Fatalf("StartEnvironmentBuild: %v", err)
	}
	if err := service.RunNextJob(context.Background()); err != nil {
		t.Fatalf("RunNextJob: %v", err)
	}
	loaded, err := store.GetEnvironmentVersion(context.Background(), version.ID)
	if err != nil || loaded.State != EnvironmentVersionReady {
		t.Fatalf("environment version = %#v, error = %v", loaded, err)
	}
	jobs, err := store.Jobs(context.Background(), value.ID)
	if err != nil || len(jobs) != 1 || jobs[0].Kind != JobKindBuildEnvironment ||
		jobs[0].State != JobStateSucceeded {
		t.Fatalf("jobs = %#v, error = %v", jobs, err)
	}
}

func TestStartEnvironmentBuildIsIdempotent(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, _ := boundEnvironmentBuildWorkspace(t, store, EnvironmentVersionPending)
	service := &Service{store: store}
	if err := service.StartEnvironmentBuild(context.Background(), value.ID); err != nil {
		t.Fatalf("StartEnvironmentBuild: %v", err)
	}
	if err := service.StartEnvironmentBuild(context.Background(), value.ID); err != nil {
		t.Fatalf("second StartEnvironmentBuild: %v", err)
	}
	jobs, err := store.Jobs(context.Background(), value.ID)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs = %#v, error = %v", jobs, err)
	}
}

func TestBuildEnvironmentJobFailureMarksVersionFailed(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, version := boundEnvironmentBuildWorkspace(t, store, EnvironmentVersionPending)
	service := &Service{
		store: store,
		runtime: &fakeRuntime{shell: &recordingShell{
			result: exec.ShellResult{ExitCode: 1, Stderr: "npm not found"},
		}},
		git: &recordingGit{},
	}
	if err := service.StartEnvironmentBuild(context.Background(), value.ID); err != nil {
		t.Fatalf("StartEnvironmentBuild: %v", err)
	}
	if err := service.RunNextJob(context.Background()); err != nil {
		t.Fatalf("RunNextJob: %v", err)
	}
	loaded, err := store.GetEnvironmentVersion(context.Background(), version.ID)
	if err != nil || loaded.State != EnvironmentVersionFailed ||
		!strings.Contains(loaded.Error, "npm not found") {
		t.Fatalf("environment version = %#v, error = %v", loaded, err)
	}
	jobs, err := store.Jobs(context.Background(), value.ID)
	if err != nil || len(jobs) != 1 || jobs[0].State != JobStateFailed {
		t.Fatalf("jobs = %#v, error = %v", jobs, err)
	}
}

func TestStartEnvironmentBuildRejectsUnboundWorkspace(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value := environmentProjectWorkspace(t, store, "unbound")
	service := &Service{store: store}
	err = service.StartEnvironmentBuild(context.Background(), value.ID)
	if err == nil || !strings.Contains(err.Error(), "not bound") {
		t.Fatalf("StartEnvironmentBuild error = %v", err)
	}
}

func TestStartEnvironmentBuildRejectsReadyVersion(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, _ := boundEnvironmentBuildWorkspace(t, store, EnvironmentVersionReady)
	service := &Service{store: store}
	err = service.StartEnvironmentBuild(context.Background(), value.ID)
	if err == nil || !strings.Contains(err.Error(), "already ready") {
		t.Fatalf("StartEnvironmentBuild error = %v", err)
	}
}

func boundEnvironmentBuildWorkspace(
	t *testing.T, store *Store, state string,
) (Workspace, EnvironmentVersion) {
	t.Helper()
	value := environmentProjectWorkspace(t, store, "build")
	environment, err := store.FindOrCreateEnvironment(context.Background(), "owner/project", ".")
	if err != nil {
		t.Fatalf("FindOrCreateEnvironment: %v", err)
	}
	spec := EnvironmentSpec{ProjectRoot: ".", Fingerprint: "build-fingerprint"}
	version, err := store.CreateEnvironmentVersion(context.Background(), environment.ID,
		spec.Fingerprint, spec, workspaceCachePath(value.Path))
	if err != nil {
		t.Fatalf("CreateEnvironmentVersion: %v", err)
	}
	if state == EnvironmentVersionReady {
		if err := store.SetEnvironmentVersionState(context.Background(), version.ID,
			EnvironmentVersionReady, ""); err != nil {
			t.Fatalf("SetEnvironmentVersionState: %v", err)
		}
	}
	if err := store.BindWorkspaceEnvironment(context.Background(), value.ID, version.ID); err != nil {
		t.Fatalf("BindWorkspaceEnvironment: %v", err)
	}
	profile := ProjectProfile{
		ProjectRoot: ".", SetupCommand: "npm ci", EnvironmentSpec: &spec,
	}
	if err := store.SaveProfile(context.Background(), value.ID, profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	return value, version
}
