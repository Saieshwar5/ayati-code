package workspace

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/exec"
	"github.com/Saieshwar5/perpetual/internal/workspaceruntime"
)

func TestServiceInitializeStartsRuntime(t *testing.T) {
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
	runtime := &fakeRuntime{shell: &recordingShell{result: exec.ShellResult{ExitCode: 0}}}
	service := &Service{store: store, runtime: runtime, git: &recordingGit{}}
	if err := service.Initialize(context.Background(), value.ID); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if len(runtime.started) != 1 || runtime.started[0].ID != value.ID {
		t.Fatalf("started refs = %#v", runtime.started)
	}
}

func TestServiceStopAndResumeRouteRuntimeLifecycle(t *testing.T) {
	store, value := readyWorkspace(t, "perpetual/change", true)
	runtime := &fakeRuntime{}
	service := &Service{store: store, runtime: runtime, git: &recordingGit{}}
	if err := service.Stop(context.Background(), value.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	loaded, err := store.Get(context.Background(), value.ID)
	if err != nil || loaded.Status != StatusStopped {
		t.Fatalf("workspace after stop = %#v, error = %v", loaded, err)
	}
	if len(runtime.stopped) != 1 || runtime.stopped[0].ID != value.ID {
		t.Fatalf("stopped refs = %#v", runtime.stopped)
	}
	if err := service.Resume(context.Background(), value.ID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	loaded, err = store.Get(context.Background(), value.ID)
	if err != nil || loaded.Status != StatusReady {
		t.Fatalf("workspace after resume = %#v, error = %v", loaded, err)
	}
	if len(runtime.started) != 1 || runtime.started[0].ID != value.ID {
		t.Fatalf("started refs = %#v", runtime.started)
	}
}

func TestServiceArchiveStopsRuntime(t *testing.T) {
	store, value := readyWorkspace(t, "main", false)
	runtime := &fakeRuntime{}
	service := &Service{store: store, runtime: runtime}
	if err := service.Archive(context.Background(), value.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if len(runtime.stopped) != 1 || runtime.stopped[0].ID != value.ID {
		t.Fatalf("stopped refs = %#v", runtime.stopped)
	}
}

func TestServiceDeleteDestroysRuntime(t *testing.T) {
	root, store, value := deletionWorkspace(t)
	runtime := &fakeRuntime{}
	service := &Service{store: store, runtime: runtime, git: &recordingGit{}, root: root}
	if err := service.Delete(context.Background(), value.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(runtime.destroyed) != 1 || runtime.destroyed[0].ID != value.ID {
		t.Fatalf("destroyed refs = %#v", runtime.destroyed)
	}
	if _, err := store.Get(context.Background(), value.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("workspace record still exists: %v", err)
	}
}

func TestServiceReportsRuntimeStartFailure(t *testing.T) {
	store, value := readyWorkspace(t, "stopped", false)
	if err := store.UpdateStatus(context.Background(), value.ID, StatusStopped, ""); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	runtime := &fakeRuntime{startErr: errors.New("runtime unavailable")}
	service := &Service{store: store, runtime: runtime, git: &recordingGit{}}
	if err := service.Resume(context.Background(), value.ID); err == nil ||
		!strings.Contains(err.Error(), "start workspace runtime") {
		t.Fatalf("Resume error = %v", err)
	}
}

func TestServiceReportsRuntimeDestroyFailure(t *testing.T) {
	root, store, value := deletionWorkspace(t)
	runtime := &fakeRuntime{destroyErr: errors.New("runtime stuck")}
	service := &Service{store: store, runtime: runtime, git: &recordingGit{}, root: root}
	if err := service.Delete(context.Background(), value.ID); err == nil ||
		!strings.Contains(err.Error(), "destroy workspace runtime") {
		t.Fatalf("Delete error = %v", err)
	}
	loaded, _ := store.Get(context.Background(), value.ID)
	if loaded.Status != StatusDeletionFailed {
		t.Fatalf("workspace = %#v", loaded)
	}
	if len(runtime.destroyed) != 1 || len(runtime.started) != 0 || len(runtime.stopped) != 0 {
		t.Fatalf("runtime refs = %#v", runtime)
	}
}

func TestServiceReportsRuntimeStopFailure(t *testing.T) {
	store, value := readyWorkspace(t, "main", false)
	runtime := &fakeRuntime{stopErr: errors.New("runtime busy")}
	service := &Service{store: store, runtime: runtime}
	if err := service.Stop(context.Background(), value.ID); err == nil ||
		!strings.Contains(err.Error(), "stop workspace runtime") {
		t.Fatalf("Stop error = %v", err)
	}
}

func TestServiceReportsRuntimeStartFailureDuringInitialization(t *testing.T) {
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
	runtime := &fakeRuntime{startErr: errors.New("runtime unavailable")}
	service := &Service{store: store, runtime: runtime, git: &recordingGit{}}
	if err := service.Initialize(context.Background(), value.ID); err == nil ||
		!strings.Contains(err.Error(), "start workspace runtime") {
		t.Fatalf("Initialize error = %v", err)
	}
	loaded, _ := store.Get(context.Background(), value.ID)
	if loaded.Status != StatusInitializationFailed {
		t.Fatalf("workspace = %#v", loaded)
	}
}

func TestServiceRuntimeRefMatchesWorkspacePath(t *testing.T) {
	value := Workspace{ID: "workspace-1", Path: "/managed/workspace-1/repo"}
	ref := runtimeRef(value)
	want := workspaceruntime.Ref{
		ID:        "workspace-1",
		Directory: "/managed/workspace-1/repo",
		CacheDir:  "/managed/workspace-1/cache",
	}
	if !reflect.DeepEqual(ref, want) {
		t.Fatalf("ref = %#v, want %#v", ref, want)
	}
}

type recordingRuntimeProvider struct {
	cloud workspaceruntime.Runtime
}

func (p *recordingRuntimeProvider) RuntimeFor(name string) (workspaceruntime.Runtime, error) {
	switch name {
	case "", "local":
		return workspaceruntime.NewLocal(), nil
	case "cloud":
		if p.cloud == nil {
			return nil, errors.New("cloud workspace runtime is not configured")
		}
		return p.cloud, nil
	default:
		return nil, errors.New("unknown workspace runtime " + name)
	}
}

func TestServicePersistsRuntimeStateTransitions(t *testing.T) {
	store, value := readyWorkspace(t, "perpetual/change", true)
	runtime := &fakeRuntime{}
	service := &Service{store: store, runtime: runtime, git: &recordingGit{}}
	if err := service.Stop(context.Background(), value.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	loaded, err := store.Get(context.Background(), value.ID)
	if err != nil || loaded.RuntimeState != workspaceruntime.RuntimeStateStopped {
		t.Fatalf("runtime state after stop = %q, error = %v", loaded.RuntimeState, err)
	}
	if err := service.Resume(context.Background(), value.ID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	loaded, err = store.Get(context.Background(), value.ID)
	if err != nil || loaded.RuntimeState != workspaceruntime.RuntimeStateRunning {
		t.Fatalf("runtime state after resume = %q, error = %v", loaded.RuntimeState, err)
	}
}

func TestServiceRuntimeProviderSelectsCloudAndFailsWithoutConfig(t *testing.T) {
	cloud, err := workspaceruntime.NewCloud(workspaceruntime.CloudConfig{
		Endpoint: "https://runtime.test", Token: "secret",
	})
	if err != nil {
		t.Fatalf("NewCloud: %v", err)
	}
	service := &Service{provider: &recordingRuntimeProvider{cloud: cloud}}
	selected, err := service.runtimeFor(Workspace{RuntimeProvider: "cloud"})
	if err != nil || selected != cloud {
		t.Fatalf("selected runtime = %#v, error = %v", selected, err)
	}
	local, err := service.runtimeFor(Workspace{})
	if err != nil || local == nil {
		t.Fatalf("local runtime = %#v, error = %v", local, err)
	}
	unconfigured := &Service{provider: &recordingRuntimeProvider{}}
	_, err = unconfigured.runtimeFor(Workspace{RuntimeProvider: "cloud"})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("unconfigured runtime error = %v", err)
	}
}
