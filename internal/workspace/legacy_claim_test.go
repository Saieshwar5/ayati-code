package workspace

import (
	"context"
	"path/filepath"
	"testing"
)

func TestClaimLegacyRowsAssignsUnownedWorkspacesJobsAndEnvironments(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	root := t.TempDir()

	legacy, err := store.Create(ctx, Create{
		Repository: "owner/legacy", CloneURL: "https://github.com/owner/legacy.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(root, "legacy", "repo"),
	})
	if err != nil {
		t.Fatalf("Create legacy workspace: %v", err)
	}
	if legacy.UserID != "" {
		t.Fatalf("legacy workspace owner = %q", legacy.UserID)
	}
	if _, err := store.CreateJob(ctx, legacy.ID, JobKindPrepare); err != nil {
		t.Fatalf("CreateJob legacy: %v", err)
	}
	if _, err := store.FindOrCreateEnvironment(ctx, "", "owner/legacy", "."); err != nil {
		t.Fatalf("FindOrCreateEnvironment legacy: %v", err)
	}
	owned, err := store.Create(ctx, Create{
		UserID: "user-a", Repository: "owner/legacy",
		CloneURL:   "https://github.com/owner/legacy.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(root, "owned", "repo"),
	})
	if err != nil {
		t.Fatalf("Create owned workspace: %v", err)
	}
	if err := store.SaveProfile(ctx, owned.ID, ProjectProfile{
		ProjectRoot: ".", SetupCommand: "npm ci",
	}); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	claim, err := store.ClaimLegacyRows(ctx, "user-a")
	if err != nil {
		t.Fatalf("ClaimLegacyRows: %v", err)
	}
	if claim.Workspaces < 1 || claim.Jobs < 1 || claim.Environments < 1 {
		t.Fatalf("legacy claim = %#v", claim)
	}
	loaded, err := store.Get(ctx, legacy.ID)
	if err != nil || loaded.UserID != "user-a" {
		t.Fatalf("claimed workspace = %#v, error = %v", loaded, err)
	}
	jobs, err := store.Jobs(ctx, legacy.ID)
	if err != nil || len(jobs) != 1 || jobs[0].UserID != "user-a" {
		t.Fatalf("claimed jobs = %#v, error = %v", jobs, err)
	}
	if _, err := store.FindOrCreateEnvironment(ctx, "user-a", "owner/legacy", "."); err != nil {
		t.Fatalf("reopen claimed environment: %v", err)
	}
}
