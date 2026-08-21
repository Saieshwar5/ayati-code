package workspace

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestStoreScopesWorkspacesByOwner(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	root := t.TempDir()
	for _, owner := range []string{"user-a", "user-b"} {
		if _, err := store.Create(ctx, Create{
			UserID: owner, Repository: "owner/" + owner,
			CloneURL:   "https://github.com/owner/" + owner + ".git",
			BaseBranch: "main", Branch: "main",
			Path: filepath.Join(root, owner, "repo"),
		}); err != nil {
			t.Fatalf("Create workspace for %s: %v", owner, err)
		}
	}
	values, err := store.ListForUser(ctx, "user-a")
	if err != nil || len(values) != 1 || values[0].UserID != "user-a" {
		t.Fatalf("ListForUser user-a = %#v, error = %v", values, err)
	}
	ownedA := values[0].ID
	values, err = store.ListForUser(ctx, "user-b")
	if err != nil || len(values) != 1 || values[0].UserID != "user-b" {
		t.Fatalf("ListForUser user-b = %#v, error = %v", values, err)
	}
	if _, err := store.GetForUser(ctx, "user-a", ownedA); err != nil {
		t.Fatalf("GetForUser owner: %v", err)
	}
	if _, err := store.GetForUser(ctx, "user-b", ownedA); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetForUser other owner error = %v", err)
	}
	if len(store.mustWorkspaceIDsForUser(ctx, "user-a")) != 1 {
		t.Fatalf("user-a owner rows did not match")
	}
}

func (s *Store) mustWorkspaceIDsForUser(ctx context.Context, userID string) []string {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM workspaces WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			panic(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestStoreScopesEnvironmentsByOwner(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	first, err := store.FindOrCreateEnvironment(ctx, "user-a", "owner/project", ".")
	if err != nil {
		t.Fatalf("FindOrCreateEnvironment user-a: %v", err)
	}
	second, err := store.FindOrCreateEnvironment(ctx, "user-b", "owner/project", ".")
	if err != nil {
		t.Fatalf("FindOrCreateEnvironment user-b: %v", err)
	}
	if first.ID == second.ID || first.UserID != "user-a" || second.UserID != "user-b" {
		t.Fatalf("environment isolation failed: %#v vs %#v", first, second)
	}
	spec := EnvironmentSpec{ProjectRoot: ".", Fingerprint: "shared-fingerprint"}
	version, err := store.CreateEnvironmentVersion(ctx, first.ID, spec.Fingerprint, spec, "cache")
	if err != nil {
		t.Fatalf("CreateEnvironmentVersion: %v", err)
	}
	if err := store.SetEnvironmentVersionState(ctx, version.ID, EnvironmentVersionReady, ""); err != nil {
		t.Fatalf("SetEnvironmentVersionState: %v", err)
	}
	if found, ok, err := store.FindReadyEnvironmentVersion(ctx, "user-a", first.ID, spec.Fingerprint); err != nil || !ok || found.ID != version.ID {
		t.Fatalf("owner-ready version = %#v, ok = %v, error = %v", found, ok, err)
	}
	if _, ok, err := store.FindReadyEnvironmentVersion(ctx, "user-b", first.ID, spec.Fingerprint); err != nil || ok {
		t.Fatalf("cross-owner ready version ok = %v, error = %v", ok, err)
	}
}

func TestStoreRecordsJobOwner(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	root := t.TempDir()
	first, err := store.Create(ctx, Create{
		UserID: "user-a", Repository: "owner/project-a",
		CloneURL:   "https://github.com/owner/project-a.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(root, "a", "repo"),
	})
	if err != nil {
		t.Fatalf("Create user-a: %v", err)
	}
	second, err := store.Create(ctx, Create{
		UserID: "user-b", Repository: "owner/project-b",
		CloneURL:   "https://github.com/owner/project-b.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(root, "b", "repo"),
	})
	if err != nil {
		t.Fatalf("Create user-b: %v", err)
	}
	jobA, err := store.CreateJob(ctx, first.ID, JobKindPrepare)
	if err != nil {
		t.Fatalf("CreateJob user-a: %v", err)
	}
	jobB, err := store.CreateJob(ctx, second.ID, JobKindPrepare)
	if err != nil {
		t.Fatalf("CreateJob user-b: %v", err)
	}
	if jobA.UserID != "user-a" || jobB.UserID != "user-b" {
		t.Fatalf("job owners = %q and %q", jobA.UserID, jobB.UserID)
	}
	claimed, err := store.ClaimNextJob(ctx)
	if err != nil {
		t.Fatalf("ClaimNextJob: %v", err)
	}
	if claimed.UserID != "user-a" && claimed.UserID != "user-b" {
		t.Fatalf("claimed job owner = %q", claimed.UserID)
	}
}
