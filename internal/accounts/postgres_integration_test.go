package accounts

import (
	"context"
	"os"
	"testing"
	"time"

	appdatabase "github.com/Saieshwar5/perpetual/internal/database"
)

// TestAccountsPostgres exercises the accounts store against a real Postgres
// database. It is skipped unless PERPETUAL_TEST_POSTGRES_URL is set so local
// unit runs and CI without a database remain fast.
func TestAccountsPostgres(t *testing.T) {
	dsn := os.Getenv("PERPETUAL_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("set PERPETUAL_TEST_POSTGRES_URL to run the Postgres integration test")
	}
	ctx := context.Background()
	database, err := appdatabase.OpenConfigured(ctx, appdatabase.Config{
		Provider: appdatabase.ProviderPostgres,
		URL:      dsn,
	})
	if err != nil {
		t.Fatalf("OpenConfigured: %v", err)
	}
	defer database.Close()

	store, err := NewStore(database)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	user, err := store.UpsertGitHubUser(ctx, 424242, "postgres-test-user", "Postgres Test", "https://example.com/avatar")
	if err != nil {
		t.Fatalf("UpsertGitHubUser: %v", err)
	}
	if user.Login != "postgres-test-user" {
		t.Fatalf("login = %q", user.Login)
	}

	if err := store.SaveGitHubCredential(ctx, user.ID, "ghp_postgres_secret"); err != nil {
		t.Fatalf("SaveGitHubCredential: %v", err)
	}
	if token, err := store.GitHubCredential(ctx, user.ID); err != nil || token != "ghp_postgres_secret" {
		t.Fatalf("GitHubCredential = %q, %v", token, err)
	}

	session, err := store.CreateSession(ctx, user.ID, "opaque-token", time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if session.ID == "" {
		t.Fatal("session ID is empty")
	}
	found, ok, err := store.UserBySessionToken(ctx, "opaque-token")
	if err != nil || !ok {
		t.Fatalf("UserBySessionToken = %#v, %v", found, err)
	}
}
