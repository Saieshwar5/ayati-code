package webapp

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/ayati-code/internal/agent"
	"github.com/Saieshwar5/ayati-code/internal/workspace"
)

func TestHandlerManagesAgentsAndSelectsOneForSession(t *testing.T) {
	handler, store, _, _ := testHandler(t)
	response := serve(handler, http.MethodGet, "/api/agents", "", false)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), agent.BuiltinAgentID) {
		t.Fatalf("list agents status = %d, body = %s", response.Code, response.Body.String())
	}
	create := `{"name":"Test specialist","emoji":"🧪","description":"Improves tests","provider_id":"fireworks","model":"test-model","max_steps":8,"shell_enabled":true,"instructions":"Inspect failures first."}`
	response = serve(handler, http.MethodPost, "/api/agents", create, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("create agent status = %d, body = %s", response.Code, response.Body.String())
	}
	var definition agent.Definition
	if err := json.Unmarshal(response.Body.Bytes(), &definition); err != nil {
		t.Fatalf("decode agent: %v", err)
	}
	response = serve(handler, http.MethodPost, "/api/agents/"+definition.ID+"/default", "", true)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"default":true`) {
		t.Fatalf("default agent status = %d, body = %s", response.Code, response.Body.String())
	}
	value, err := store.Create(context.Background(), workspace.Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	sessions, err := store.ListSessions(context.Background(), value.ID)
	if err != nil || len(sessions) != 1 || sessions[0].SelectedAgentID != definition.ID {
		t.Fatalf("sessions = %#v, error = %v", sessions, err)
	}
	path := "/api/workspaces/" + value.ID + "/sessions/" + sessions[0].ID
	response = serve(handler, http.MethodPatch, path,
		`{"agent_id":"`+agent.BuiltinAgentID+`"}`, true)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), agent.BuiltinAgentID) {
		t.Fatalf("select session agent status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodPatch, "/api/agents/"+agent.BuiltinAgentID, create, true)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "cannot be edited") {
		t.Fatalf("built-in update status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestHandlerListsProvidersAndRejectsUnknownAgentProvider(t *testing.T) {
	handler, _, _, _ := testHandler(t)
	response := serve(handler, http.MethodGet, "/api/providers", "", false)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"fireworks"`) ||
		!strings.Contains(response.Body.String(), `"configured":true`) {
		t.Fatalf("providers status = %d, body = %s", response.Code, response.Body.String())
	}
	create := `{"name":"Unknown provider","provider_id":"missing","max_steps":8}`
	response = serve(handler, http.MethodPost, "/api/agents", create, true)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "provider is not available") {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
}
