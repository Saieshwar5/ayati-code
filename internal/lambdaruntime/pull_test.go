package lambdaruntime

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/environments"
	"github.com/Saieshwar5/perpetual/internal/vmagent"
	"github.com/Saieshwar5/perpetual/internal/workspace"
)

func TestPullRepoCollectsWorkingTree(t *testing.T) {
	vmRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(vmRoot, "note.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatalf("write vm file: %v", err)
	}
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
		UserID: "user-1", Repository: "owner/pull", CloneURL: "https://github.com/owner/pull.git",
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
	if err := store.SaveRuntimeInstance(ctx, workspace.RuntimeInstance{
		WorkspaceID: ws.ID, Provider: "lambda", InstanceID: "vm-1", Endpoint: server.URL, State: "RUNNING",
	}); err != nil {
		t.Fatalf("SaveRuntimeInstance: %v", err)
	}
	scratch := t.TempDir()
	if err := runtime.PullRepo(ctx, ws.ID, scratch); err != nil {
		t.Fatalf("PullRepo: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(scratch, "note.txt"))
	if err != nil || string(got) != "changed" {
		t.Fatalf("pulled note = %q, %v", got, err)
	}
}
