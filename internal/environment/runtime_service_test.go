package environment_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/environment"
)

type fakeRuntimeDriver struct {
	created     []environment.RuntimeSpec
	destroyed   []environment.RuntimeSpec
	destroyedID []string
	createErr   error
	destroyErr  error
}

func (f *fakeRuntimeDriver) Create(
	_ context.Context, spec environment.RuntimeSpec,
) (environment.Runtime, error) {
	f.created = append(f.created, spec)
	if f.createErr != nil {
		return environment.Runtime{}, f.createErr
	}
	identity := strings.Repeat(string(rune('a'+len(f.created)-1)), 64)
	return environment.Runtime{
		ID: identity, Name: "runtime", EnvironmentID: spec.Environment.ID,
		WorkspaceID: spec.Lease.WorkspaceID, LeaseID: spec.Lease.ID,
		Generation: spec.Lease.Generation, Running: true,
	}, nil
}

func TestRuntimeServiceReleasesInterruptedAcquisition(t *testing.T) {
	_, workspaces, store := openStores(t)
	compute := createReadyEnvironment(t, store, "Interrupted")
	project := createWorkspace(t, workspaces, "owner/interrupted", "interrupted")
	lease, err := store.Acquire(context.Background(), project.ID, compute.ID)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	driver := &fakeRuntimeDriver{}
	service, err := environment.NewRuntimeService(store, driver)
	if err != nil {
		t.Fatalf("NewRuntimeService: %v", err)
	}
	if err := service.Stop(context.Background(), environment.StopInput{
		WorkspaceID: project.ID, WorkspacePath: project.Path, CachePath: project.Path + "-cache",
	}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(driver.destroyed) != 1 || driver.destroyed[0].Lease.State != environment.LeaseReleasing ||
		driver.destroyedID[0] != "" {
		t.Fatalf("destroyed = %#v, IDs = %#v", driver.destroyed, driver.destroyedID)
	}
	if _, err := store.ActiveForWorkspace(context.Background(), project.ID); err == nil {
		t.Fatal("interrupted lease remains active")
	}
	available, err := store.Get(context.Background(), compute.ID)
	if err != nil || available.State != environment.StateAvailable || available.Generation != lease.Generation {
		t.Fatalf("environment = %#v, error = %v", available, err)
	}
}

func (f *fakeRuntimeDriver) Destroy(
	_ context.Context, spec environment.RuntimeSpec, runtimeID string,
) error {
	f.destroyed = append(f.destroyed, spec)
	f.destroyedID = append(f.destroyedID, runtimeID)
	return f.destroyErr
}

func TestRuntimeServiceActivatesAndReleasesExactLease(t *testing.T) {
	_, workspaces, store := openStores(t)
	compute := createReadyEnvironment(t, store, "General coding")
	project := createWorkspace(t, workspaces, "owner/runtime", "runtime")
	driver := &fakeRuntimeDriver{}
	service, err := environment.NewRuntimeService(store, driver)
	if err != nil {
		t.Fatalf("NewRuntimeService: %v", err)
	}
	input := environment.StartInput{
		WorkspaceID: project.ID, PreferredEnvironmentID: compute.ID,
		WorkspacePath: project.Path, CachePath: project.Path + "-cache",
	}
	assignment, err := service.Start(context.Background(), input)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if assignment.Lease.State != environment.LeaseActive || assignment.Lease.RuntimeID == "" ||
		assignment.Runtime.EnvironmentID != compute.ID || len(driver.created) != 1 {
		t.Fatalf("assignment = %#v, created = %#v", assignment, driver.created)
	}
	active, err := store.ActiveForWorkspace(context.Background(), project.ID)
	if err != nil || active.RuntimeID != assignment.Runtime.ID || active.Generation != assignment.Lease.Generation {
		t.Fatalf("active lease = %#v, error = %v", active, err)
	}
	if err := service.Stop(context.Background(), environment.StopInput{
		WorkspaceID: project.ID, WorkspacePath: project.Path,
		CachePath: project.Path + "-cache",
	}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(driver.destroyed) != 1 || driver.destroyedID[0] != assignment.Runtime.ID ||
		driver.destroyed[0].Lease.State != environment.LeaseReleasing {
		t.Fatalf("destroyed = %#v, IDs = %#v", driver.destroyed, driver.destroyedID)
	}
	available, err := store.Get(context.Background(), compute.ID)
	if err != nil || available.State != environment.StateAvailable || available.ActiveLease != nil {
		t.Fatalf("environment = %#v, error = %v", available, err)
	}
}

func TestRuntimeServiceCleanupIsIdempotentWithoutActiveLease(t *testing.T) {
	_, workspaces, store := openStores(t)
	compute := createReadyEnvironment(t, store, "Deletion cleanup")
	project := createWorkspace(t, workspaces, "owner/delete", "delete")
	driver := &fakeRuntimeDriver{}
	service, err := environment.NewRuntimeService(store, driver)
	if err != nil {
		t.Fatalf("NewRuntimeService: %v", err)
	}
	_, err = service.Start(context.Background(), environment.StartInput{
		WorkspaceID: project.ID, PreferredEnvironmentID: compute.ID,
		WorkspacePath: project.Path, CachePath: project.Path + "-cache",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	input := environment.StopInput{
		WorkspaceID: project.ID, WorkspacePath: project.Path, CachePath: project.Path + "-cache",
	}
	if err := service.Cleanup(context.Background(), input); err != nil {
		t.Fatalf("first Cleanup: %v", err)
	}
	if err := service.Cleanup(context.Background(), input); err != nil {
		t.Fatalf("second Cleanup: %v", err)
	}
	if len(driver.destroyed) != 1 {
		t.Fatalf("destroyed runtimes = %#v", driver.destroyed)
	}
}

func TestRuntimeServiceQuarantinesCreateFailure(t *testing.T) {
	_, workspaces, store := openStores(t)
	compute := createReadyEnvironment(t, store, "Broken creation")
	project := createWorkspace(t, workspaces, "owner/broken", "broken")
	driver := &fakeRuntimeDriver{createErr: errors.New("Docker refused the policy")}
	service, err := environment.NewRuntimeService(store, driver)
	if err != nil {
		t.Fatalf("NewRuntimeService: %v", err)
	}
	_, err = service.Start(context.Background(), environment.StartInput{
		WorkspaceID: project.ID, PreferredEnvironmentID: compute.ID,
		WorkspacePath: project.Path, CachePath: project.Path + "-cache",
	})
	if err == nil || !strings.Contains(err.Error(), "Docker refused the policy") {
		t.Fatalf("Start error = %v", err)
	}
	failed, getErr := store.Get(context.Background(), compute.ID)
	if getErr != nil || failed.State != environment.StateFailed || failed.ActiveLease != nil ||
		!strings.Contains(failed.Error, "Docker refused the policy") {
		t.Fatalf("environment = %#v, error = %v", failed, getErr)
	}
	if err := service.Cleanup(context.Background(), environment.StopInput{
		WorkspaceID: project.ID, WorkspacePath: project.Path, CachePath: project.Path + "-cache",
	}); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if len(driver.destroyed) != 1 || driver.destroyed[0].Lease.State != environment.LeaseFailed {
		t.Fatalf("destroyed = %#v", driver.destroyed)
	}
}

func TestRuntimeServiceQuarantinesDestroyFailure(t *testing.T) {
	_, workspaces, store := openStores(t)
	compute := createReadyEnvironment(t, store, "Broken release")
	project := createWorkspace(t, workspaces, "owner/release", "release")
	driver := &fakeRuntimeDriver{destroyErr: errors.New("container remains")}
	service, err := environment.NewRuntimeService(store, driver)
	if err != nil {
		t.Fatalf("NewRuntimeService: %v", err)
	}
	_, err = service.Start(context.Background(), environment.StartInput{
		WorkspaceID: project.ID, PreferredEnvironmentID: compute.ID,
		WorkspacePath: project.Path, CachePath: project.Path + "-cache",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	err = service.Stop(context.Background(), environment.StopInput{
		WorkspaceID: project.ID, WorkspacePath: project.Path, CachePath: project.Path + "-cache",
	})
	if err == nil || !strings.Contains(err.Error(), "container remains") {
		t.Fatalf("Stop error = %v", err)
	}
	failed, getErr := store.Get(context.Background(), compute.ID)
	if getErr != nil || failed.State != environment.StateFailed || failed.ActiveLease != nil {
		t.Fatalf("environment = %#v, error = %v", failed, getErr)
	}
}
