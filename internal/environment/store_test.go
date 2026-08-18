package environment_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	appdatabase "github.com/Saieshwar5/perpetual/internal/database"
	"github.com/Saieshwar5/perpetual/internal/environment"
	"github.com/Saieshwar5/perpetual/internal/workspace"
)

func TestStoreCreatesAndProvisionsEnvironment(t *testing.T) {
	_, _, store := openStores(t)
	ctx := context.Background()
	created, err := store.Create(ctx, environment.CreateInput{Name: "General coding", ImageRef: "perpetual/development:latest"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Driver != environment.DriverDocker || created.CPUMillis != 2000 || created.MemoryMB != 4096 ||
		created.PIDLimit != 256 || created.NetworkPolicy != environment.NetworkOutbound ||
		created.ProvisioningState != environment.Provisioning || created.State != environment.StateProvisioning {
		t.Fatalf("environment = %#v", created)
	}
	if err := store.MarkReady(ctx, created.ID, "sha256:resolved"); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	loaded, err := store.Get(ctx, created.ID)
	if err != nil || loaded.State != environment.StateAvailable || loaded.ImageDigest != "sha256:resolved" {
		t.Fatalf("environment = %#v, error = %v", loaded, err)
	}
	values, err := store.List(ctx)
	if err != nil || len(values) != 1 || values[0].ID != created.ID {
		t.Fatalf("environments = %#v, error = %v", values, err)
	}
	if _, err := store.Create(ctx, environment.CreateInput{Name: "General coding", ImageRef: "duplicate"}); err == nil {
		t.Fatal("Create accepted a duplicate name")
	}
}

func TestStoreMaintainsExactLeaseLifecycle(t *testing.T) {
	_, workspaces, store := openStores(t)
	ctx := context.Background()
	compute := createReadyEnvironment(t, store, "General coding")
	project := createWorkspace(t, workspaces, "owner/project", "project")
	lease, err := store.Acquire(ctx, project.ID, compute.ID)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if lease.State != environment.LeaseAcquiring || lease.Generation != 1 {
		t.Fatalf("lease = %#v", lease)
	}
	occupied, err := store.Get(ctx, compute.ID)
	if err != nil || occupied.State != environment.StateOccupied || occupied.ActiveLease == nil ||
		occupied.ActiveLease.WorkspaceID != project.ID {
		t.Fatalf("occupied environment = %#v, error = %v", occupied, err)
	}
	if err := store.Activate(ctx, lease.ID, lease.Generation, "runtime-1"); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if err := store.BeginRelease(ctx, lease.ID, lease.Generation+1); !errors.Is(err, environment.ErrLeaseState) {
		t.Fatalf("stale BeginRelease error = %v", err)
	}
	if err := store.BeginRelease(ctx, lease.ID, lease.Generation); err != nil {
		t.Fatalf("BeginRelease: %v", err)
	}
	releasing, err := store.Get(ctx, compute.ID)
	if err != nil || releasing.State != environment.StateReleasing {
		t.Fatalf("releasing environment = %#v, error = %v", releasing, err)
	}
	if err := store.CompleteRelease(ctx, lease.ID, lease.Generation); err != nil {
		t.Fatalf("CompleteRelease: %v", err)
	}
	available, err := store.Get(ctx, compute.ID)
	if err != nil || available.State != environment.StateAvailable || available.ActiveLease != nil {
		t.Fatalf("available environment = %#v, error = %v", available, err)
	}
	nextProject := createWorkspace(t, workspaces, "owner/next", "next")
	nextLease, err := store.Acquire(ctx, nextProject.ID, compute.ID)
	if err != nil || nextLease.Generation != lease.Generation+1 {
		t.Fatalf("next lease = %#v, error = %v", nextLease, err)
	}
	secondEnvironment := createReadyEnvironment(t, store, "Second environment")
	if _, err := store.Acquire(ctx, nextProject.ID, secondEnvironment.ID); !errors.Is(err, environment.ErrWorkspaceLeased) {
		t.Fatalf("duplicate workspace lease error = %v", err)
	}
}

func TestStoreLeasesEnvironmentExclusively(t *testing.T) {
	_, workspaces, store := openStores(t)
	ctx := context.Background()
	createReadyEnvironment(t, store, "Only environment")
	const attempts = 8
	workspaceIDs := make([]string, attempts)
	for index := range workspaceIDs {
		workspaceIDs[index] = createWorkspace(t, workspaces,
			fmt.Sprintf("owner/project-%d", index), fmt.Sprintf("project-%d", index)).ID
	}
	start := make(chan struct{})
	results := make(chan error, attempts)
	var wait sync.WaitGroup
	for _, workspaceID := range workspaceIDs {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := store.Acquire(ctx, workspaceID, "")
			results <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, environment.ErrNoEnvironmentAvailable) {
			t.Fatalf("Acquire error = %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful acquisitions = %d, want 1", successes)
	}
}

func TestStorePreventsDeletingOccupiedRelationships(t *testing.T) {
	_, workspaces, store := openStores(t)
	ctx := context.Background()
	compute := createReadyEnvironment(t, store, "Protected")
	project := createWorkspace(t, workspaces, "owner/protected", "protected")
	lease, err := store.Acquire(ctx, project.ID, compute.ID)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := store.Delete(ctx, compute.ID); !errors.Is(err, environment.ErrEnvironmentOccupied) {
		t.Fatalf("Delete environment error = %v", err)
	}
	if err := workspaces.Delete(ctx, project.ID); err == nil {
		t.Fatal("workspace deletion ignored active environment lease")
	}
	if err := store.Fail(ctx, lease.ID, lease.Generation, errors.New("runtime creation failed")); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	failed, err := store.Get(ctx, compute.ID)
	if err != nil || failed.State != environment.StateFailed || failed.Error != "runtime creation failed" {
		t.Fatalf("failed environment = %#v, error = %v", failed, err)
	}
	if err := workspaces.Delete(ctx, project.ID); err != nil {
		t.Fatalf("Delete workspace after failed lease: %v", err)
	}
	if err := store.Delete(ctx, compute.ID); err != nil {
		t.Fatalf("Delete environment after failed lease: %v", err)
	}
	if _, err := store.Get(ctx, compute.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("Get deleted environment error = %v", err)
	}
}

func openStores(t *testing.T) (*appdatabase.Database, *workspace.Store, *environment.Store) {
	t.Helper()
	database, err := appdatabase.Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	workspaces, err := workspace.NewStore(database)
	if err != nil {
		t.Fatalf("workspace.NewStore: %v", err)
	}
	store, err := environment.NewStore(database)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return database, workspaces, store
}

func createReadyEnvironment(t *testing.T, store *environment.Store, name string) environment.Environment {
	t.Helper()
	value, err := store.Create(context.Background(), environment.CreateInput{Name: name, ImageRef: "perpetual/development:latest"})
	if err != nil {
		t.Fatalf("Create environment: %v", err)
	}
	if err := store.MarkReady(context.Background(), value.ID, "sha256:"+value.ID); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	value, err = store.Get(context.Background(), value.ID)
	if err != nil {
		t.Fatalf("Get environment: %v", err)
	}
	return value
}

func createWorkspace(t *testing.T, store *workspace.Store, repository, directory string) workspace.Workspace {
	t.Helper()
	value, err := store.Create(context.Background(), workspace.Create{
		Repository: repository, CloneURL: "https://github.com/" + repository + ".git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), directory),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	return value
}
