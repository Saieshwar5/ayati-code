package environment_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/environment"
)

type fakeImageResolver struct {
	digest string
	err    error
	calls  []string
}

func (f *fakeImageResolver) ResolveImage(_ context.Context, image string) (string, error) {
	f.calls = append(f.calls, image)
	return f.digest, f.err
}

func TestManagementServiceCreatesRepairsAndDeletesCapacity(t *testing.T) {
	_, _, store := openStores(t)
	resolver := &fakeImageResolver{err: errors.New("image is missing")}
	service, err := environment.NewManagementService(store, resolver)
	if err != nil {
		t.Fatalf("NewManagementService: %v", err)
	}
	created, err := service.Create(context.Background(), environment.CreateInput{
		Name: "Node projects", ImageRef: "perpetual/node:dev", MemoryMB: 2048,
	})
	if err == nil || created.State != environment.StateFailed ||
		!strings.Contains(created.Error, "image is missing") {
		t.Fatalf("created = %#v, error = %v", created, err)
	}
	resolver.err = nil
	resolver.digest = "sha256:" + strings.Repeat("a", 64)
	repaired, err := service.Repair(context.Background(), created.ID)
	if err != nil || repaired.State != environment.StateAvailable ||
		repaired.ImageDigest != resolver.digest || len(resolver.calls) != 2 {
		t.Fatalf("repaired = %#v, calls = %#v, error = %v", repaired, resolver.calls, err)
	}
	values, err := service.List(context.Background())
	if err != nil || len(values) != 1 || values[0].ID != created.ID {
		t.Fatalf("values = %#v, error = %v", values, err)
	}
	if _, err := service.Repair(context.Background(), created.ID); err == nil ||
		!strings.Contains(err.Error(), "does not need repair") {
		t.Fatalf("ready Repair error = %v", err)
	}
	if err := service.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(context.Background(), created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("Get deleted environment error = %v", err)
	}
}

func TestManagementServiceKeepsFailedRuntimeLeaseQuarantined(t *testing.T) {
	_, workspaces, store := openStores(t)
	value := createReadyEnvironment(t, store, "Quarantined")
	project := createWorkspace(t, workspaces, "owner/quarantined", "quarantined")
	lease, err := store.Acquire(context.Background(), project.ID, value.ID)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := store.Fail(context.Background(), lease.ID, lease.Generation,
		errors.New("runtime removal is uncertain")); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	service, err := environment.NewManagementService(store,
		&fakeImageResolver{digest: "sha256:" + strings.Repeat("b", 64)})
	if err != nil {
		t.Fatalf("NewManagementService: %v", err)
	}
	if _, err := service.Repair(context.Background(), value.ID); !errors.Is(err, environment.ErrEnvironmentQuarantined) {
		t.Fatalf("Repair error = %v", err)
	}
	if err := service.Delete(context.Background(), value.ID); !errors.Is(err, environment.ErrEnvironmentQuarantined) {
		t.Fatalf("Delete error = %v", err)
	}
	loaded, err := store.Get(context.Background(), value.ID)
	if err != nil || !loaded.Quarantined {
		t.Fatalf("quarantined environment = %#v, error = %v", loaded, err)
	}
}
