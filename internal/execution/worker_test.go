package execution

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Saieshwar5/perpetual/internal/workspace"
	"github.com/Saieshwar5/perpetual/internal/workspaceruntime"
)

func TestRunWorkerDrainsQueuedRunWithStubProvider(t *testing.T) {
	store, err := workspace.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ws, err := store.Create(ctx, workspace.Create{
		UserID: "user-w", Repository: "owner/worker", CloneURL: "https://github.com/owner/worker.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	session, err := store.CreateSession(ctx, ws.ID, "worker session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	run, err := store.EnqueueRun(ctx, workspace.EnqueueRunInput{
		UserID: "user-w", WorkspaceID: ws.ID, SessionID: session.ID, MaxSteps: 5,
	})
	if err != nil {
		t.Fatalf("EnqueueRun: %v", err)
	}

	factory := func(_ workspace.Run) (ShellRunner, error) {
		return NewRuntimeShell(workspaceruntime.NewLocal(),
			workspaceruntime.Ref{ID: ws.ID, Directory: ws.Path},
			map[string]string{"PATH": os.Getenv("PATH")})
	}
	worker, err := NewWorkerWithFactory(store, StubProvider{}, factory)
	if err != nil {
		t.Fatalf("NewWorkerWithFactory: %v", err)
	}
	go RunWorker(ctx, worker, 20*time.Millisecond)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, err := store.GetRun(ctx, run.ID)
		if err == nil && current.State == workspace.RunCompleted {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("run did not complete within deadline")
}
