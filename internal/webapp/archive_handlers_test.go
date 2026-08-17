package webapp

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/ayati-code/internal/workspace"
)

func TestHandlerArchivesAndRestoresWorkspace(t *testing.T) {
	handler, store, _, _ := testHandler(t)
	value, err := store.Create(context.Background(), workspace.Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	response := serve(handler, http.MethodPost, "/api/workspaces/"+value.ID+"/archive", "", true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("archive status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodGet, "/api/workspaces", "", false)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), value.ID) {
		t.Fatalf("active list status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodGet, "/api/workspaces?archived=true", "", false)
	var archived []workspace.Workspace
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &archived) != nil ||
		len(archived) != 1 || archived[0].ArchivedAt == nil {
		t.Fatalf("archived list status = %d, body = %s", response.Code, response.Body.String())
	}

	response = serve(handler, http.MethodPost, "/api/workspaces/"+value.ID+"/restore", "", true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("restore status = %d, body = %s", response.Code, response.Body.String())
	}
	loaded, err := store.Get(context.Background(), value.ID)
	if err != nil || loaded.ArchivedAt != nil || loaded.Status != workspace.StatusStopped {
		t.Fatalf("restored workspace = %#v, error = %v", loaded, err)
	}
}

func TestHandlerServesApplicationRoutes(t *testing.T) {
	handler, _, _, _ := testHandler(t)
	for _, path := range []string{
		"/workspaces", "/workspaces/new", "/workspaces/archived", "/workspaces/workspace-1",
		"/workspaces/workspace-1/sessions/session-1", "/agents", "/environments",
	} {
		response := serve(handler, http.MethodGet, path, "", false)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `id="root"`) {
			t.Fatalf("GET %s status = %d, body = %s", path, response.Code, response.Body.String())
		}
	}
}
