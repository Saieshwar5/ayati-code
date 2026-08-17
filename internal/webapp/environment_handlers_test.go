package webapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	compute "github.com/Saieshwar5/ayati-code/internal/environment"
)

type fakeEnvironmentManagement struct {
	values []compute.Environment
}

func (f *fakeEnvironmentManagement) List(context.Context) ([]compute.Environment, error) {
	return append([]compute.Environment(nil), f.values...), nil
}

func (f *fakeEnvironmentManagement) Create(
	_ context.Context, input compute.CreateInput,
) (compute.Environment, error) {
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.ImageRef) == "" {
		return compute.Environment{}, errors.New("environment name and image are required")
	}
	value := compute.Environment{
		ID: "environment-new", Name: input.Name, Driver: compute.DriverDocker,
		ImageRef: input.ImageRef, ImageDigest: "sha256:" + strings.Repeat("a", 64),
		CPUMillis: input.CPUMillis, MemoryMB: input.MemoryMB, PIDLimit: input.PIDLimit,
		NetworkPolicy: input.NetworkPolicy, ProvisioningState: compute.ProvisioningReady,
		State: compute.StateAvailable, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	f.values = append(f.values, value)
	return value, nil
}

func (f *fakeEnvironmentManagement) Repair(
	_ context.Context, id string,
) (compute.Environment, error) {
	if id == "missing" {
		return compute.Environment{}, sql.ErrNoRows
	}
	if id == "ready" {
		return compute.Environment{ID: id, ProvisioningState: compute.ProvisioningReady},
			compute.ErrEnvironmentReady
	}
	if id == "quarantined" {
		return compute.Environment{ID: id, ProvisioningState: compute.ProvisioningFailed},
			compute.ErrEnvironmentQuarantined
	}
	return compute.Environment{ID: id, State: compute.StateAvailable}, nil
}

func (f *fakeEnvironmentManagement) Delete(_ context.Context, id string) error {
	if id == "occupied" {
		return compute.ErrEnvironmentOccupied
	}
	if id == "missing" {
		return sql.ErrNoRows
	}
	if id == "quarantined" {
		return compute.ErrEnvironmentQuarantined
	}
	return nil
}

func TestEnvironmentHandlersManageReusableCapacity(t *testing.T) {
	handler, _, _, _ := testHandler(t)
	response := serve(handler, http.MethodGet, "/api/environments", "", false)
	if response.Code != http.StatusOK || response.Body.String() != "[]\n" {
		t.Fatalf("list status = %d, body = %s", response.Code, response.Body.String())
	}
	input := `{"name":"Node projects","image_ref":"ayati/node:dev","cpu_millis":1500,` +
		`"memory_mb":2048,"pid_limit":128,"network_policy":"outbound"}`
	response = serve(handler, http.MethodPost, "/api/environments", input, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
	var created compute.Environment
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil ||
		created.Name != "Node projects" || created.State != compute.StateAvailable {
		t.Fatalf("created = %#v, error = %v", created, err)
	}
	response = serve(handler, http.MethodPost,
		"/api/environments/"+created.ID+"/repair", "", true)
	if response.Code != http.StatusOK {
		t.Fatalf("repair status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodDelete, "/api/environments/"+created.ID, "", true)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestEnvironmentHandlersGuardMutationsAndOccupiedCapacity(t *testing.T) {
	handler, _, _, _ := testHandler(t)
	response := serve(handler, http.MethodPost, "/api/environments", `{}`, false)
	if response.Code != http.StatusForbidden {
		t.Fatalf("unguarded create status = %d", response.Code)
	}
	response = serve(handler, http.MethodPost, "/api/environments", `{}`, true)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid create status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodDelete, "/api/environments/occupied", "", true)
	if response.Code != http.StatusConflict {
		t.Fatalf("occupied delete status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodDelete, "/api/environments/missing", "", true)
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing delete status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodPost, "/api/environments/ready/repair", "", true)
	if response.Code != http.StatusConflict {
		t.Fatalf("ready repair status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodPost, "/api/environments/quarantined/repair", "", true)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "failed workspace") {
		t.Fatalf("quarantined repair status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodDelete, "/api/environments/quarantined", "", true)
	if response.Code != http.StatusConflict {
		t.Fatalf("quarantined delete status = %d, body = %s", response.Code, response.Body.String())
	}
}
