package execution

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/workspace"
	"github.com/Saieshwar5/perpetual/internal/workspaceruntime"
)

// TestEndToEndLocalSmoke verifies the full vertical slice on the local
// runtime: workspace -> durable run -> model -> shell tool -> file change ->
// durable steps -> completed run.
func TestEndToEndLocalSmoke(t *testing.T) {
	store, err := workspace.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	workspacePath := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		t.Fatalf("MkdirAll workspace path: %v", err)
	}

	ws, err := store.Create(ctx, workspace.Create{
		UserID: "e2e-user", Repository: "owner/prototype",
		CloneURL:   "https://github.com/owner/prototype.git",
		BaseBranch: "main", Branch: "feature/e2e", CreateBranch: true,
		Path: workspacePath,
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	session, err := store.CreateSession(ctx, ws.ID, "prototype session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	run, err := store.EnqueueRun(ctx, workspace.EnqueueRunInput{
		UserID: "e2e-user", WorkspaceID: ws.ID, SessionID: session.ID,
		Prompt: "Please create a note file", MaxSteps: 10,
	})
	if err != nil {
		t.Fatalf("EnqueueRun: %v", err)
	}

	provider := &fakeProvider{responses: []ModelResponse{
		{ToolCalls: []ToolCall{{Name: "shell", Arguments: map[string]any{"command": "printf 'hello prototype' > note.txt"}}}},
		{StopReason: "stop", Content: "done"},
	}}
	ref := workspaceruntime.Ref{ID: ws.ID, Directory: workspacePath}
	shell, err := NewRuntimeShell(workspaceruntime.NewLocal(), ref, map[string]string{"PATH": os.Getenv("PATH")})
	if err != nil {
		t.Fatalf("NewRuntimeShell: %v", err)
	}
	worker, err := NewWorker(store, provider, shell)
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	if err := worker.WorkOnce(ctx); err != nil {
		t.Fatalf("WorkOnce: %v", err)
	}

	completed, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if completed.State != workspace.RunCompleted {
		t.Fatalf("run state = %q, want completed", completed.State)
	}

	steps, err := store.RunSteps(ctx, run.ID)
	if err != nil {
		t.Fatalf("RunSteps: %v", err)
	}
	if len(steps) < 3 {
		t.Fatalf("steps = %d, want model+shell+model", len(steps))
	}

	note, err := os.ReadFile(filepath.Join(workspacePath, "note.txt"))
	if err != nil {
		t.Fatalf("read note.txt: %v", err)
	}
	if strings.TrimSpace(string(note)) != "hello prototype" {
		t.Fatalf("note.txt = %q", string(note))
	}
}

// TestEndToEndEmptyQueue ensures an empty queue is not an error at the loop
// boundary and keeps the worker alive.
func TestEndToEndEmptyQueue(t *testing.T) {
	store, err := workspace.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	shell, err := NewRuntimeShell(workspaceruntime.NewLocal(),
		workspaceruntime.Ref{ID: "none", Directory: t.TempDir()},
		map[string]string{"PATH": os.Getenv("PATH")})
	if err != nil {
		t.Fatalf("NewRuntimeShell: %v", err)
	}
	worker, err := NewWorker(store, &fakeProvider{}, shell)
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	if err := worker.WorkOnce(context.Background()); !errors.Is(err, errNoRuns) {
		t.Fatalf("WorkOnce error = %v, want ErrNoRuns", err)
	}
}
