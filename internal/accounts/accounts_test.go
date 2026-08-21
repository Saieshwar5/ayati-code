package accounts

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appdatabase "github.com/Saieshwar5/perpetual/internal/database"
)

func TestUpsertGitHubUserAndSessionFlow(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	user, err := store.UpsertGitHubUser(ctx, 42, "octocat", "Octo Cat", "https://avatars.test/42")
	if err != nil {
		t.Fatalf("UpsertGitHubUser: %v", err)
	}
	updated, err := store.UpsertGitHubUser(ctx, 42, "octocat", "Octavia", "")
	if err != nil {
		t.Fatalf("UpsertGitHubUser update: %v", err)
	}
	if updated.ID != user.ID || updated.Name != "Octavia" {
		t.Fatalf("updated user = %#v, first = %#v", updated, user)
	}

	session, err := store.CreateSession(ctx, user.ID, "session-token", time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	loaded, found, err := store.UserBySessionToken(ctx, "session-token")
	if err != nil || !found || loaded.ID != user.ID {
		t.Fatalf("session lookup = %#v, found = %v, error = %v", loaded, found, err)
	}
	if _, found, err := store.UserBySessionToken(ctx, "different-token"); err != nil || found {
		t.Fatalf("other token found = %v, error = %v", found, err)
	}
	if err := store.RevokeSession(ctx, "session-token"); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if _, found, err := store.UserBySessionToken(ctx, "session-token"); err != nil || found {
		t.Fatalf("revoked session found = %v, error = %v", found, err)
	}
	if session.ExpiresAt.Before(time.Now().Add(55 * time.Minute)) {
		t.Fatalf("session expiry = %s", session.ExpiresAt)
	}
}

func TestExpiredSessionIsRejected(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	user, err := store.UpsertGitHubUser(ctx, 7, "expired", "Expired", "")
	if err != nil {
		t.Fatalf("UpsertGitHubUser: %v", err)
	}
	now := time.Now().UTC()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO auth_sessions (
		id, user_id, token_hash, created_at, expires_at
	) VALUES (?, ?, ?, ?, ?)`,
		newID(), user.ID, hashToken("expired-token"), formatTime(now.Add(-2*time.Hour)),
		formatTime(now.Add(-time.Hour))); err != nil {
		t.Fatalf("insert expired session: %v", err)
	}
	if _, found, err := store.UserBySessionToken(ctx, "expired-token"); err != nil || found {
		t.Fatalf("expired session found = %v, error = %v", found, err)
	}
}

func TestDeleteExpiredSessions(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	user, err := store.UpsertGitHubUser(ctx, 8, "cleanup", "Cleanup", "")
	if err != nil {
		t.Fatalf("UpsertGitHubUser: %v", err)
	}
	now := time.Now().UTC()
	for _, entry := range []struct {
		token string
		from  time.Duration
		to    time.Duration
	}{
		{"old-token", -2 * time.Hour, -time.Hour},
		{"fresh-token", time.Hour, 2 * time.Hour},
	} {
		if _, err := store.db.ExecContext(ctx, `INSERT INTO auth_sessions (
			id, user_id, token_hash, created_at, expires_at
		) VALUES (?, ?, ?, ?, ?)`,
			newID(), user.ID, hashToken(entry.token), formatTime(now.Add(entry.from)),
			formatTime(now.Add(entry.to))); err != nil {
			t.Fatalf("insert session: %v", err)
		}
	}
	deleted, err := store.DeleteExpiredSessions(ctx, now)
	if err != nil || deleted != 1 {
		t.Fatalf("DeleteExpiredSessions = %d, error = %v", deleted, err)
	}
	if _, found, err := store.UserBySessionToken(ctx, "old-token"); err != nil || found {
		t.Fatalf("old-token found = %v, error = %v", found, err)
	}
	if _, found, err := store.UserBySessionToken(ctx, "fresh-token"); err != nil || !found {
		t.Fatalf("fresh-token found = %v, error = %v", found, err)
	}
}

func TestGitHubCredentialRoundTripAndAtRest(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	user, err := store.UpsertGitHubUser(ctx, 99, "credential-test", "", "")
	if err != nil {
		t.Fatalf("UpsertGitHubUser: %v", err)
	}
	if err := store.SaveGitHubCredential(ctx, user.ID, "gh-secret-token"); err != nil {
		t.Fatalf("SaveGitHubCredential: %v", err)
	}
	got, err := store.GitHubCredential(ctx, user.ID)
	if err != nil || got != "gh-secret-token" {
		t.Fatalf("GitHubCredential = %q, error = %v", got, err)
	}
	var stored string
	if err := store.db.QueryRowContext(ctx,
		`SELECT ciphertext FROM github_credentials WHERE user_id = ?`, user.ID).Scan(&stored); err != nil {
		t.Fatalf("scan ciphertext: %v", err)
	}
	if strings.Contains(stored, "gh-secret-token") {
		t.Fatalf("token stored in database: %q", stored)
	}
	if err := store.RemoveGitHubCredential(ctx, user.ID); err != nil {
		t.Fatalf("RemoveGitHubCredential: %v", err)
	}
	if _, err := store.GitHubCredential(ctx, user.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("removed credential error = %v", err)
	}
}

func TestGitHubCredentialSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	database, err := appdatabase.Open(path)
	if err != nil {
		t.Fatalf("Open database: %v", err)
	}
	store, err := NewStore(database)
	if err != nil {
		database.Close()
		t.Fatalf("NewStore: %v", err)
	}
	ctx := context.Background()
	user, err := store.UpsertGitHubUser(ctx, 100, "reopen", "", "")
	if err != nil {
		database.Close()
		t.Fatalf("UpsertGitHubUser: %v", err)
	}
	if err := store.SaveGitHubCredential(ctx, user.ID, "persisted-token"); err != nil {
		database.Close()
		t.Fatalf("SaveGitHubCredential: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close database: %v", err)
	}
	database, err = appdatabase.Open(path)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer func() { _ = database.Close() }()
	store, err = NewStore(database)
	if err != nil {
		t.Fatalf("reopen NewStore: %v", err)
	}
	got, err := store.GitHubCredential(ctx, user.ID)
	if err != nil || got != "persisted-token" {
		t.Fatalf("GitHubCredential after reopen = %q, error = %v", got, err)
	}
	keyPath := filepath.Join(filepath.Dir(path), githubCredentialKeyName)
	info, err := os.Stat(keyPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("credential key permissions = %v, error = %v", info.Mode().Perm(), err)
	}
}

func testStore(t *testing.T) *Store {
	t.Helper()
	database, err := appdatabase.Open(filepath.Join(t.TempDir(), "accounts.db"))
	if err != nil {
		t.Fatalf("Open database: %v", err)
	}
	store, err := NewStore(database)
	if err != nil {
		database.Close()
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return store
}
