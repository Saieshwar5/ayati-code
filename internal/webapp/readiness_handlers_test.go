package webapp

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/workspace"
)

func TestHandlerConfiguresProjectRootAndContinuesPreparation(t *testing.T) {
	handler, store, workspaces, _ := testHandler(t)
	value, err := store.Create(context.Background(), workspace.Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	candidates := []workspace.ProjectCandidate{
		{ProjectRoot: "apps/api", Languages: []string{"Go"}, PackageManagers: []string{"Go modules"}},
		{ProjectRoot: "apps/web", Languages: []string{"Node.js"}, PackageManagers: []string{"npm"}},
	}
	if err := store.RequireProjectSelection(context.Background(), value.ID, candidates); err != nil {
		t.Fatalf("RequireProjectSelection: %v", err)
	}
	response := serve(handler, http.MethodPost, "/api/workspaces/"+value.ID+"/configure",
		`{"project_root":"apps/web"}`, true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("configure status = %d, body = %s", response.Code, response.Body.String())
	}
	if initialized := <-workspaces.initialized; initialized != value.ID {
		t.Fatalf("initialized workspace = %q", initialized)
	}
	loaded, err := store.Get(context.Background(), value.ID)
	if err != nil || loaded.SelectedProjectRoot != "apps/web" ||
		loaded.Status != workspace.StatusReady || loaded.PreparationStage != workspace.PreparationReady {
		t.Fatalf("workspace = %#v, error = %v", loaded, err)
	}
}
