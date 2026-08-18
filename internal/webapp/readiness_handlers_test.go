package webapp

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/workspace"
)

func TestHandlerDefaultsWorkspaceToProtectedExplore(t *testing.T) {
	handler, _, workspaces, _ := testHandler(t)
	create := `{"repository":"owner/project","base_branch":"main","branch":"ignored","create_branch":true}`
	response := serve(handler, http.MethodPost, "/api/workspaces", create, true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
	var value workspace.Workspace
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode workspace: %v", err)
	}
	if initialized := <-workspaces.initialized; initialized != value.ID {
		t.Fatalf("initialized workspace = %q", initialized)
	}
	if value.Authority != workspace.AuthorityExplore || value.Branch != "main" || value.CreateBranch {
		t.Fatalf("workspace = %#v", value)
	}
	publish := `{"commit_message":"feat: change","title":"Change project"}`
	response = serve(handler, http.MethodPost, "/api/workspaces/"+value.ID+"/publish", publish, true)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "Develop") {
		t.Fatalf("publish status = %d, body = %s", response.Code, response.Body.String())
	}
}

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

func TestHandlerChangesWorkspaceAuthority(t *testing.T) {
	handler, store, _, _ := testHandler(t)
	value, err := store.Create(context.Background(), workspace.Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.CompletePreparation(context.Background(), value.ID); err != nil {
		t.Fatalf("CompletePreparation: %v", err)
	}
	response := serve(handler, http.MethodPost, "/api/workspaces/"+value.ID+"/authority",
		`{"authority":"develop","branch":"perpetual/change","create_branch":true}`, true)
	if response.Code != http.StatusOK {
		t.Fatalf("authority status = %d, body = %s", response.Code, response.Body.String())
	}
	loaded, err := store.Get(context.Background(), value.ID)
	if err != nil || loaded.Authority != workspace.AuthorityDevelop ||
		loaded.Branch != "perpetual/change" || loaded.EffectiveMountMode != "rw" {
		t.Fatalf("workspace = %#v, error = %v", loaded, err)
	}
}
