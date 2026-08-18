package webapp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/workspace"
)

func TestHandlerCreatesWorkspaceOnExistingBranch(t *testing.T) {
	handler, _, workspaces, _ := testHandler(t)
	body := `{"repository":"owner/project","base_branch":"main","branch":"feature/existing",` +
		`"create_branch":false,"branch_mode":"existing","environment":[]}`
	response := serve(handler, http.MethodPost, "/api/workspaces", body, true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
	var value workspace.Workspace
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode workspace: %v", err)
	}
	if value.BaseBranch != "main" || value.Branch != "feature/existing" || value.CreateBranch {
		t.Fatalf("workspace branches = %#v", value)
	}
	<-workspaces.initialized
}

func TestHandlerAllowsDirectBranchButRejectsPublishing(t *testing.T) {
	handler, _, workspaces, _ := testHandler(t)
	body := `{"repository":"owner/project","base_branch":"main","branch":"main",` +
		`"create_branch":false,"branch_mode":"direct","environment":[]}`
	response := serve(handler, http.MethodPost, "/api/workspaces", body, true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
	var value workspace.Workspace
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode workspace: %v", err)
	}
	<-workspaces.initialized
	response = serve(handler, http.MethodPost, "/api/workspaces/"+value.ID+"/publish",
		`{"commit_message":"feat: direct","title":"Direct"}`, true)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "working branch") {
		t.Fatalf("publish status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(workspaces.published) != 0 {
		t.Fatalf("publish was called with %#v", workspaces.published)
	}
}

func TestHandlerRejectsNewBranchThatAlreadyExists(t *testing.T) {
	handler, _, _, _ := testHandler(t)
	body := `{"repository":"owner/project","base_branch":"main","branch":"feature/existing",` +
		`"create_branch":true,"branch_mode":"new","environment":[]}`
	response := serve(handler, http.MethodPost, "/api/workspaces", body, true)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "already exists") {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
}
