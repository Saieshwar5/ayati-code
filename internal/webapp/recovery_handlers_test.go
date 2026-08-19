package webapp

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/workspace"
)

func (f *fakeWorkspaceService) Resume(ctx context.Context, id string) error {
	return f.store.UpdateStatus(ctx, id, workspace.StatusReady, "")
}

func TestHandlerResumesStoppedWorkspaceWithoutInitialization(t *testing.T) {
	handler, store, workspaces, _ := testHandler(t)
	value, err := store.Create(context.Background(), workspace.Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "perpetual/change",
		Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.CompletePreparation(context.Background(), value.ID); err != nil {
		t.Fatalf("CompletePreparation: %v", err)
	}
	if err := store.UpdateStatus(context.Background(), value.ID, workspace.StatusStopped, ""); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	response := serve(handler, http.MethodPost, "/api/workspaces/"+value.ID+"/resume", "", true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("resume status = %d, body = %s", response.Code, response.Body.String())
	}
	select {
	case initialized := <-workspaces.initialized:
		t.Fatalf("resume initialized workspace %q", initialized)
	default:
	}
	loaded, err := store.Get(context.Background(), value.ID)
	if err != nil || loaded.Status != workspace.StatusReady {
		encoded, _ := json.Marshal(loaded)
		t.Fatalf("workspace = %s, error = %v", encoded, err)
	}
}
