package lambdaruntime

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/environments"
	"github.com/Saieshwar5/perpetual/internal/workspace"
	"github.com/Saieshwar5/perpetual/internal/workspaceruntime"
)

type fakeAPI struct{}

func (fakeAPI) DeleteMicrovmImageVersion(_ context.Context, _ string, _ string) error { return nil }

func (fakeAPI) CreateMicrovmImage(_ context.Context, _ environments.ImageBuildInput) (environments.ImageRef, error) {
	return environments.ImageRef{ImageARN: "arn:image", Version: "1.0", State: "CREATED"}, nil
}
func (fakeAPI) GetMicrovmImage(_ context.Context) (environments.ImageRef, error) {
	return environments.ImageRef{ImageARN: "arn:image", State: "CREATED"}, nil
}
func (fakeAPI) RunMicrovm(_ context.Context, _ environments.RunMicrovmInput) (environments.Instance, error) {
	return environments.Instance{MicrovmID: "microvm-1", Endpoint: "example.test", State: "RUNNING", ImageARN: "arn:image"}, nil
}
func (fakeAPI) AuthToken(_ context.Context, _ string) (string, error) { return "token", nil }
func (fakeAPI) SuspendMicrovm(_ context.Context, _ string) error      { return nil }
func (fakeAPI) ResumeMicrovm(_ context.Context, _ string) error       { return nil }
func (fakeAPI) TerminateMicrovm(_ context.Context, _ string) error    { return nil }
func (fakeAPI) GetMicrovm(_ context.Context, _ string) (environments.Instance, error) {
	return environments.Instance{MicrovmID: "microvm-1", Endpoint: "example.test", State: "RUNNING"}, nil
}

func TestLambdaRuntimePersistsAndRestores(t *testing.T) {
	store, err := workspace.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	ws, err := store.Create(ctx, workspace.Create{
		UserID: "user-1", Repository: "owner/lambda", CloneURL: "https://github.com/owner/lambda.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	manager, err := environments.NewManager(environments.Config{
		Region: "us-east-1", ImageARN: "arn:image", ImageVersion: "1.0",
	}, fakeAPI{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	runtime, err := NewLambda(manager, store)
	if err != nil {
		t.Fatalf("NewLambda: %v", err)
	}
	ref := workspaceruntime.Ref{ID: ws.ID, RuntimeID: "microvm-1", Directory: ws.Path}
	if err := runtime.Start(ctx, ref); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Simulate a controller restart: a fresh runtime over the same store.
	restarted, err := NewLambda(manager, store)
	if err != nil {
		t.Fatalf("NewLambda restart: %v", err)
	}
	if err := restarted.Stop(ctx, ref); err != nil {
		t.Fatalf("Stop after restart: %v", err)
	}
	if err := restarted.Resume(ctx, ref); err != nil {
		t.Fatalf("Resume after restart: %v", err)
	}
	instance, err := store.RuntimeInstance(ctx, ws.ID)
	if err != nil {
		t.Fatalf("RuntimeInstance: %v", err)
	}
	if instance.State != "running" {
		t.Fatalf("state = %q", instance.State)
	}
	if err := restarted.Destroy(ctx, ref); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if _, err := store.RuntimeInstance(ctx, ws.ID); err == nil {
		t.Fatal("expected instance deleted after destroy")
	}
}
