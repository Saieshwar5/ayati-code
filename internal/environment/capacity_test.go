package environment_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Saieshwar5/ayati-code/internal/environment"
)

func TestRuntimeServiceReusesReleasedCapacityAcrossWorkspaces(t *testing.T) {
	_, workspaces, store := openStores(t)
	createReadyEnvironment(t, store, "First environment")
	createReadyEnvironment(t, store, "Second environment")
	first := createWorkspace(t, workspaces, "owner/first", "first")
	second := createWorkspace(t, workspaces, "owner/second", "second")
	third := createWorkspace(t, workspaces, "owner/third", "third")
	driver := &fakeRuntimeDriver{}
	service, err := environment.NewRuntimeService(store, driver)
	if err != nil {
		t.Fatalf("NewRuntimeService: %v", err)
	}

	firstAssignment := startWorkspace(t, service, first.ID, first.Path)
	secondAssignment := startWorkspace(t, service, second.ID, second.Path)
	if firstAssignment.Lease.EnvironmentID == secondAssignment.Lease.EnvironmentID {
		t.Fatalf("workspaces share environment %s", firstAssignment.Lease.EnvironmentID)
	}

	_, err = service.Start(context.Background(), environment.StartInput{
		WorkspaceID: third.ID, WorkspacePath: third.Path, CachePath: third.Path + "-cache",
	})
	if !errors.Is(err, environment.ErrNoEnvironmentAvailable) {
		t.Fatalf("third Start error = %v, want no capacity", err)
	}

	if err := service.Stop(context.Background(), environment.StopInput{
		WorkspaceID: first.ID, WorkspacePath: first.Path, CachePath: first.Path + "-cache",
	}); err != nil {
		t.Fatalf("Stop first workspace: %v", err)
	}
	thirdAssignment := startWorkspace(t, service, third.ID, third.Path)
	if thirdAssignment.Lease.EnvironmentID != firstAssignment.Lease.EnvironmentID ||
		thirdAssignment.Lease.Generation != firstAssignment.Lease.Generation+1 {
		t.Fatalf("first = %#v, reused = %#v", firstAssignment.Lease, thirdAssignment.Lease)
	}
}

func startWorkspace(
	t *testing.T,
	service *environment.RuntimeService,
	workspaceID string,
	workspacePath string,
) environment.Assignment {
	t.Helper()
	assignment, err := service.Start(context.Background(), environment.StartInput{
		WorkspaceID:   workspaceID,
		WorkspacePath: workspacePath,
		CachePath:     workspacePath + "-cache",
	})
	if err != nil {
		t.Fatalf("Start workspace %s: %v", workspaceID, err)
	}
	return assignment
}
