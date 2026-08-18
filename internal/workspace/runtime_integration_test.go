package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/agent"
	appdatabase "github.com/Saieshwar5/perpetual/internal/database"
	compute "github.com/Saieshwar5/perpetual/internal/environment"
	"github.com/Saieshwar5/perpetual/internal/sandbox"
)

func TestWorkspaceLeaseRuntimeIntegration(t *testing.T) {
	if os.Getenv("PERPETUAL_DOCKER_INTEGRATION") != "1" && os.Getenv("AYATI_DOCKER_INTEGRATION") != "1" {
		t.Skip("set PERPETUAL_DOCKER_INTEGRATION=1 to exercise Docker")
	}
	ctx := context.Background()
	database, err := appdatabase.Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	store, err := NewStore(database)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	environments, err := compute.NewStore(database)
	if err != nil {
		t.Fatalf("environment.NewStore: %v", err)
	}
	driver, err := sandbox.NewDockerDriver()
	if err != nil {
		t.Fatalf("NewDockerDriver: %v", err)
	}
	digest, err := driver.ResolveImage(ctx, sandbox.DefaultImage)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	capacity, err := environments.Create(ctx, compute.CreateInput{
		Name: "Workspace integration", ImageRef: sandbox.DefaultImage,
		CPUMillis: 1000, MemoryMB: 1024, PIDLimit: 128, NetworkPolicy: compute.NetworkDisabled,
	})
	if err != nil {
		t.Fatalf("Create environment: %v", err)
	}
	if err := environments.MarkReady(ctx, capacity.ID, digest); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	runtime, err := sandbox.NewRuntimeManager(environments, driver)
	if err != nil {
		t.Fatalf("NewRuntimeManager: %v", err)
	}
	root := t.TempDir()
	value, err := store.Create(ctx, Create{
		Repository: "owner/integration", CloneURL: "https://github.com/owner/integration.git",
		BaseBranch: "main", Branch: "main", Root: root,
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	service := &Service{store: store, environment: runtime, git: &recordingGit{}, root: root}
	t.Cleanup(func() {
		_ = runtime.Stop(context.Background(), runtimeInput(value))
	})
	if err := service.Initialize(ctx, value.ID); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	firstLease, err := environments.ActiveForWorkspace(ctx, value.ID)
	if err != nil || firstLease.State != compute.LeaseActive || firstLease.RuntimeID == "" {
		t.Fatalf("first lease = %#v, error = %v", firstLease, err)
	}
	shell, loaded, err := service.Shell(ctx, value.ID)
	if err != nil {
		t.Fatalf("Shell: workspace = %#v, error = %v", loaded, err)
	}
	result := shell.Execute(ctx, agent.ShellRequest{
		Command: "touch allowed.txt && touch /cache/prepared && printf ready",
	})
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, "ready") {
		t.Fatalf("Shell result = %#v", result)
	}
	if err := service.Stop(ctx, value.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	available, err := environments.Get(ctx, capacity.ID)
	if err != nil || available.State != compute.StateAvailable || available.ActiveLease != nil {
		t.Fatalf("available environment = %#v, error = %v", available, err)
	}
	if err := service.Resume(ctx, value.ID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	secondLease, err := environments.ActiveForWorkspace(ctx, value.ID)
	if err != nil || secondLease.Generation != firstLease.Generation+1 ||
		secondLease.ID == firstLease.ID || secondLease.RuntimeID == firstLease.RuntimeID {
		t.Fatalf("second lease = %#v, error = %v", secondLease, err)
	}
	if _, err := os.Stat(filepath.Join(value.Path, "allowed.txt")); err != nil {
		t.Fatalf("preserved workspace file: %v", err)
	}
	if err := service.Stop(ctx, value.ID); err != nil {
		t.Fatalf("final Stop: %v", err)
	}
}
