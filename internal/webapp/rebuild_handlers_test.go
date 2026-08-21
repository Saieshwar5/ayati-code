package webapp

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/workspace"
)

func TestHandlerRebuildsBoundEnvironment(t *testing.T) {
	handler, store, _, _ := testHandler(t)
	value, err := store.Create(context.Background(), workspace.Create{
		UserID:     testAccountUserID,
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	environment, err := store.FindOrCreateEnvironment(context.Background(), "owner/project", ".")
	if err != nil {
		t.Fatalf("FindOrCreateEnvironment: %v", err)
	}
	version, err := store.CreateEnvironmentVersion(context.Background(), environment.ID,
		"fingerprint", workspace.EnvironmentSpec{Fingerprint: "fingerprint"}, "")
	if err != nil {
		t.Fatalf("CreateEnvironmentVersion: %v", err)
	}
	if err := store.BindWorkspaceEnvironment(context.Background(), value.ID, version.ID); err != nil {
		t.Fatalf("BindWorkspaceEnvironment: %v", err)
	}
	response := serve(handler, http.MethodPost,
		"/api/workspaces/"+value.ID+"/environment/rebuild", "", true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("rebuild status = %d, body = %s", response.Code, response.Body.String())
	}
}
