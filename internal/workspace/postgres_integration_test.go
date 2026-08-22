//go:build integration

package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	appdatabase "github.com/Saieshwar5/perpetual/internal/database"
)

// TestWorkspaceStorePostgres exercises the workspace store against a real
// Postgres database. It runs only with the integration build tag and when
// PERPETUAL_TEST_POSTGRES_URL is set.
func TestWorkspaceStorePostgres(t *testing.T) {
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
	defer store.Close()

	workspace, err := store.Create(ctx, Create{
		UserID:       "user-integration",
		Repository:   "owner/project",
		CloneURL:     "https://github.com/owner/project.git",
		BaseBranch:   "main",
		Branch:       "work-1",
		CreateBranch: true,
		Setup:        "echo setup",
		Path:         filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	if workspace.ID == "" {
		t.Fatal("workspace ID is empty")
	}

	got, err := store.Get(ctx, workspace.ID)
	if err != nil {
		t.Fatalf("Get workspace: %v", err)
	}
	if got.Repository != "owner/project" {
		t.Fatalf("repository = %q", got.Repository)
	}

	// Environment values: ciphertext round trip through BYTEA on Postgres.
	if _, err := store.UpsertEnvironment(ctx, workspace.ID, EnvironmentInput{
		Name: "TOKEN", Value: "s3cr3t" + workspace.ID, ExposeDuringSetup: true,
	}); err != nil {
		t.Fatalf("UpsertEnvironment: %v", err)
	}
	values, err := store.EnvironmentValues(ctx, workspace.ID, true)
	if err != nil {
		t.Fatalf("EnvironmentValues: %v", err)
	}
	if values["TOKEN"] != "s3cr3t"+workspace.ID {
		t.Fatalf("environment token = %q", values["TOKEN"])
	}

	// Sessions + messages use the identity-backed messages table.
	session, err := store.CreateSession(ctx, workspace.ID, "Original session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := store.AppendMessage(ctx, session.ID, Message{Role: "user", Content: "hello"}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	messages, err := store.Messages(ctx, session.ID)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("message count = %d", len(messages))
	}

	// Durable jobs.
	job, err := store.CreateJob(ctx, workspace.ID, "prepare_workspace")
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	active, err := store.HasActiveJob(ctx, workspace.ID, "prepare_workspace")
	if err != nil {
		t.Fatalf("HasActiveJob: %v", err)
	}
	if !active {
		t.Fatal("expected active job")
	}
	if err := store.FinishJob(ctx, job.ID, "succeeded", ""); err != nil {
		t.Fatalf("FinishJob: %v", err)
	}
}
