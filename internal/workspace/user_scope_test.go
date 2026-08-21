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
