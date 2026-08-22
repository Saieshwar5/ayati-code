package lambdaruntime

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/environments"
	"github.com/Saieshwar5/perpetual/internal/vmagent"
	"github.com/Saieshwar5/perpetual/internal/workspace"
)

type pushAPI struct{ endpoint string }

func (p pushAPI) CreateMicrovmImage(_ context.Context, _ environments.ImageBuildInput) (environments.ImageRef, error) {
	return environments.ImageRef{}, nil
}
func (p pushAPI) DeleteMicrovmImageVersion(_ context.Context, _ string, _ string) error { return nil }

func (p pushAPI) GetMicrovmImage(_ context.Context) (environments.ImageRef, error) {
	return environments.ImageRef{}, nil
}
func (p pushAPI) RunMicrovm(_ context.Context, _ environments.RunMicrovmInput) (environments.Instance, error) {
	return environments.Instance{MicrovmID: "vm-1", Endpoint: p.endpoint, State: "RUNNING"}, nil
}
func (p pushAPI) AuthToken(_ context.Context, _ string) (string, error) { return "token", nil }
func (p pushAPI) SuspendMicrovm(_ context.Context, _ string) error      { return nil }
func (p pushAPI) ResumeMicrovm(_ context.Context, _ string) error       { return nil }
func (p pushAPI) TerminateMicrovm(_ context.Context, _ string) error    { return nil }

func (p pushAPI) GetMicrovm(_ context.Context, _ string) (environments.Instance, error) {
	return environments.Instance{MicrovmID: "vm-1", Endpoint: p.endpoint, State: "RUNNING"}, nil
}

func TestPushRepoBootstrapsWorkingTree(t *testing.T) {
	vmRoot := t.TempDir()
	handler := &vmagent.Handler{Root: vmRoot}
	server := httptest.NewServer(handler.DataHandler())
	defer server.Close()

	store, err := workspace.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	ws, err := store.Create(ctx, workspace.Create{
		UserID: "user-1", Repository: "owner/push", CloneURL: "https://github.com/owner/push.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	manager, err := environments.NewManager(environments.Config{Region: "us-east-1", ImageARN: "arn:image"}, pushAPI{endpoint: server.URL})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	runtime, err := NewLambda(manager, store)
	if err != nil {
		t.Fatalf("NewLambda: %v", err)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := store.SaveRuntimeInstance(ctx, workspace.RuntimeInstance{
		WorkspaceID: ws.ID, Provider: "lambda", InstanceID: "vm-1", Endpoint: server.URL, State: "RUNNING",
	}); err != nil {
		t.Fatalf("SaveRuntimeInstance: %v", err)
	}
	if err := runtime.PushRepo(ctx, ws.ID, source); err != nil {
		t.Fatalf("PushRepo: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(vmRoot, "README.md"))
	if err != nil || string(got) != "hi" {
		t.Fatalf("pushed README = %q, %v", got, err)
	}
	_ = bytes.MinRead
}
