package workspace

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRuntimeInstanceLifecycle(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	ws, err := store.Create(ctx, Create{
		UserID: "user-1", Repository: "owner/runtime", CloneURL: "https://github.com/owner/runtime.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	instance := RuntimeInstance{
		WorkspaceID: ws.ID, Provider: "lambda", InstanceID: "microvm-1",
		Endpoint: "example.test", ImageARN: "arn:image", State: "RUNNING",
	}
	if err := store.SaveRuntimeInstance(ctx, instance); err != nil {
		t.Fatalf("SaveRuntimeInstance: %v", err)
	}
	instance.State = "suspended"
	if err := store.SaveRuntimeInstance(ctx, instance); err != nil {
		t.Fatalf("SaveRuntimeInstance update: %v", err)
	}
	got, err := store.RuntimeInstance(ctx, ws.ID)
	if err != nil {
		t.Fatalf("RuntimeInstance: %v", err)
	}
	if got.InstanceID != "microvm-1" || got.State != "suspended" {
		t.Fatalf("got = %#v", got)
	}
	all, err := store.ListRuntimeInstances(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("list = %#v, err = %v", all, err)
	}
	if err := store.DeleteRuntimeInstance(ctx, ws.ID); err != nil {
		t.Fatalf("DeleteRuntimeInstance: %v", err)
	}
	if _, err := store.RuntimeInstance(ctx, ws.ID); err == nil {
		t.Fatal("expected no rows after delete")
	}
}
