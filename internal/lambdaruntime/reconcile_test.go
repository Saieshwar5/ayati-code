package lambdaruntime

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/environments"
	"github.com/Saieshwar5/perpetual/internal/workspace"
	"github.com/Saieshwar5/perpetual/internal/workspaceruntime"
)

type stateFakeAPI struct{ state string }

func (f stateFakeAPI) CreateMicrovmImage(_ context.Context, _ environments.ImageBuildInput) (environments.ImageRef, error) {
	return environments.ImageRef{}, nil
}
func (f stateFakeAPI) DeleteMicrovmImageVersion(_ context.Context, _ string, _ string) error {
	return nil
}

func (f stateFakeAPI) GetMicrovmImage(_ context.Context) (environments.ImageRef, error) {
	return environments.ImageRef{}, nil
}
func (f stateFakeAPI) RunMicrovm(_ context.Context, _ environments.RunMicrovmInput) (environments.Instance, error) {
	return environments.Instance{}, nil
}
func (f stateFakeAPI) AuthToken(_ context.Context, _ string) (string, error) { return "", nil }
func (f stateFakeAPI) SuspendMicrovm(_ context.Context, _ string) error      { return nil }
func (f stateFakeAPI) ResumeMicrovm(_ context.Context, _ string) error       { return nil }
func (f stateFakeAPI) TerminateMicrovm(_ context.Context, _ string) error    { return nil }
func (f stateFakeAPI) GetMicrovm(_ context.Context, _ string) (environments.Instance, error) {
	return environments.Instance{State: f.state}, nil
}

func TestReconcileDeletesTerminatedAndKeepsRunning(t *testing.T) {
	store, err := workspace.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	keep := t.TempDir()
	wsRunning, err := store.Create(ctx, workspace.Create{
		UserID: "user-1", Repository: "owner/running", CloneURL: "https://github.com/owner/running.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(keep, "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	wsDead, err := store.Create(ctx, workspace.Create{
		UserID: "user-2", Repository: "owner/dead", CloneURL: "https://github.com/owner/dead.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	for _, record := range []workspace.RuntimeInstance{
		{WorkspaceID: wsRunning.ID, Provider: "lambda", InstanceID: "vm-running", State: "RUNNING"},
		{WorkspaceID: wsDead.ID, Provider: "lambda", InstanceID: "vm-dead", State: "RUNNING"},
	} {
		if err := store.SaveRuntimeInstance(ctx, record); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	manager, err := environments.NewManager(environments.Config{
		Region: "us-east-1", ImageARN: "arn:image",
	}, stateFakeAPI{state: "TERMINATED"})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	runtime, err := NewLambda(manager, store)
	if err != nil {
		t.Fatalf("NewLambda: %v", err)
	}
	if err := runtime.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// GetMicrovm always returns TERMINATED, so both records are removed here.
	for _, wsID := range []string{wsRunning.ID, wsDead.ID} {
		if _, err := store.RuntimeInstance(ctx, wsID); err == nil {
			t.Fatalf("workspace %s still has an instance record", wsID)
		}
	}
	_ = workspaceruntime.Ref{}
}
