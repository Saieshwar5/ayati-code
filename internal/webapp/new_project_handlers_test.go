package webapp

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/githubapp"
	"github.com/Saieshwar5/perpetual/internal/workspace"
)

func (f *fakeGitHub) CreateRepository(
	_ context.Context, _ string, input githubapp.CreateRepositoryInput,
) (githubapp.Repository, error) {
	f.created = append(f.created, input)
	if f.createError != nil {
		return githubapp.Repository{}, f.createError
	}
	return githubapp.Repository{
		ID: 10, FullName: "octocat/" + input.Name,
		CloneURL:      "https://github.com/octocat/" + input.Name + ".git",
		DefaultBranch: "trunk", Private: input.Private,
	}, nil
}

func TestHandlerCreatesPrivateWorkspaceForNewProject(t *testing.T) {
	handler, _, workspaces, github := testHandler(t)
	response := serve(handler, http.MethodPost, "/api/workspaces/new-project",
		`{"name":"new-project","description":"A new project","branch":"perpetual/initial","setup_command":"","environment":[]}`, true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
	var created workspace.Workspace
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode workspace: %v", err)
	}
	if initialized := <-workspaces.initialized; initialized != created.ID {
		t.Fatalf("initialized workspace = %q", initialized)
	}
	if len(github.created) != 1 || !github.created[0].Private || created.Repository != "octocat/new-project" ||
		created.BaseBranch != "trunk" || created.Branch != "perpetual/initial" || !created.CreateBranch {
		t.Fatalf("created repository = %#v, workspace = %#v", github.created, created)
	}
}

func TestHandlerCreatesPublicWorkspaceOnLocalBranch(t *testing.T) {
	handler, _, workspaces, github := testHandler(t)
	response := serve(handler, http.MethodPost, "/api/workspaces/new-project",
		`{"name":"public-project","private":false,"branch":"perpetual/initial","environment":[]}`, true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
	var created workspace.Workspace
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode workspace: %v", err)
	}
	<-workspaces.initialized
	if github.created[0].Private || created.Branch != "perpetual/initial" || !created.CreateBranch {
		t.Fatalf("created repository = %#v, workspace = %#v", github.created, created)
	}
}

func TestHandlerExplainsMissingRepositoryPermission(t *testing.T) {
	handler, _, _, github := testHandler(t)
	github.createError = githubapp.APIError{StatusCode: http.StatusForbidden, Status: "403 Forbidden"}
	response := serve(handler, http.MethodPost, "/api/workspaces/new-project",
		`{"name":"new-project","branch":"perpetual/initial","environment":[]}`, true)
	if response.Code != http.StatusForbidden ||
		response.Body.String() != "{\"error\":\"GitHub App needs Administration: write permission to create repositories\"}\n" {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
}
