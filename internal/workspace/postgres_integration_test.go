//go:build integration

package workspace

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
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

// TestPostgresWorkersClaimDistinctJobs verifies that several worker goroutines
// never claim the same job when Postgres SKIP LOCKED is used.
func TestPostgresWorkersClaimDistinctJobs(t *testing.T) {
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
		UserID:     "worker-test-user",
		Repository: "owner/jobs",
		CloneURL:   "https://github.com/owner/jobs.git",
		BaseBranch: "main",
		Branch:     "main",
		Path:       filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}

	const jobCount = 6
	expected := make(map[string]bool, jobCount)
	for i := 0; i < jobCount; i++ {
		job, err := store.CreateJob(ctx, workspace.ID, "prepare_workspace")
		if err != nil {
			t.Fatalf("CreateJob %d: %v", i, err)
		}
		expected[job.ID] = true
	}

	var (
		mu      sync.Mutex
		claimed = make(map[string]bool)
		errs    = make(chan error, 4)
		wg      sync.WaitGroup
	)
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				job, err := store.ClaimNextJob(ctx)
				if err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						return
					}
					errs <- err
					return
				}
				mu.Lock()
				claimed[job.ID] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("claim error: %v", err)
	}
	if len(claimed) != jobCount {
		t.Fatalf("claimed %d jobs, expected %d", len(claimed), jobCount)
	}
	for id := range expected {
		if !claimed[id] {
			t.Fatalf("job %s was never claimed", id)
		}
	}
}

// TestPostgresWorkersClaimDistinctRuns verifies execution rooms are claimed
// exactly once when several workers race on Postgres.
func TestPostgresWorkersClaimDistinctRuns(t *testing.T) {
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
		UserID:     "run-worker-user",
		Repository: "owner/runs",
		CloneURL:   "https://github.com/owner/runs.git",
		BaseBranch: "main",
		Branch:     "main",
		Path:       filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	session, err := store.CreateSession(ctx, workspace.ID, "run claims")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	const runCount = 6
	expected := make(map[string]bool, runCount)
	for i := 0; i < runCount; i++ {
		run, err := store.EnqueueRun(ctx, EnqueueRunInput{
			UserID: "run-worker-user", WorkspaceID: workspace.ID, SessionID: session.ID,
			MaxSteps: 50,
		})
		if err != nil {
			t.Fatalf("EnqueueRun %d: %v", i, err)
		}
		expected[run.ID] = true
	}

	var (
		mu      sync.Mutex
		claimed = make(map[string]bool)
		errs    = make(chan error, 4)
		wg      sync.WaitGroup
	)
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				run, err := store.ClaimNextRun(ctx)
				if err != nil {
					if errors.Is(err, ErrNoRuns) || errors.Is(err, sql.ErrNoRows) {
						return
					}
					errs <- err
					return
				}
				mu.Lock()
				claimed[run.ID] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("claim run error: %v", err)
	}
	if len(claimed) != runCount {
		t.Fatalf("claimed %d runs, expected %d", len(claimed), runCount)
	}
	for id := range expected {
		if !claimed[id] {
			t.Fatalf("run %s was never claimed", id)
		}
	}
}
