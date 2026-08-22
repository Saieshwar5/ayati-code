package execution

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/workspace"
	"github.com/Saieshwar5/perpetual/internal/workspaceruntime"
)

// fakeProvider returns the scripted responses in order.
type fakeProvider struct {
	responses []ModelResponse
	index     int
}

func (f *fakeProvider) Complete(_ context.Context, _ ModelRequest) (ModelResponse, error) {
	if f.index >= len(f.responses) {
		return ModelResponse{}, errors.New("no more scripted responses")
	}
	response := f.responses[f.index]
	f.index++
	return response, nil
}

func TestWorkerExecutesShellToolAndCompletesRun(t *testing.T) {
	store, err := workspace.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	ws, err := store.Create(ctx, workspace.Create{
		UserID: "user-1", Repository: "owner/exec-project",
		CloneURL:   "https://github.com/owner/exec-project.git",
		BaseBranch: "main", Branch: "main",
		Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	session, err := store.CreateSession(ctx, ws.ID, "execution room")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	run, err := store.EnqueueRun(ctx, workspace.EnqueueRunInput{
		UserID: "user-1", WorkspaceID: ws.ID, SessionID: session.ID, MaxSteps: 10,
	})
	if err != nil {
		t.Fatalf("EnqueueRun: %v", err)
	}

	provider := &fakeProvider{responses: []ModelResponse{
		{StopReason: "", ToolCalls: []ToolCall{{Name: "shell", Arguments: map[string]any{"command": "echo hi"}}}},
		{StopReason: "stop", Content: "done"},
	}}
	if err := os.MkdirAll(ws.Path, 0o700); err != nil {
		t.Fatalf("MkdirAll workspace path: %v", err)
	}
	ref := workspaceruntime.Ref{ID: ws.ID, Directory: ws.Path}
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
		t.Fatalf("steps = %#v, want model+shell+model", steps)
	}
	var sawShell bool
	for _, step := range steps {
		if step.Kind == workspace.StepShell && step.Output["stdout"] == "hi\n" {
			sawShell = true
		}
	}
	if !sawShell {
		t.Fatalf("no shell step with stdout=hi found in %#v", steps)
	}
}

func TestWorkerReturnsErrNoRuns(t *testing.T) {
	store, err := workspace.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	provider := &fakeProvider{}
	shell, err := NewRuntimeShell(workspaceruntime.NewLocal(),
		workspaceruntime.Ref{ID: "none", Directory: t.TempDir()},
		map[string]string{"PATH": os.Getenv("PATH")})
	if err != nil {
		t.Fatalf("NewRuntimeShell: %v", err)
	}
	worker, err := NewWorker(store, provider, shell)
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	if err := worker.WorkOnce(context.Background()); !errors.Is(err, errNoRuns) {
		t.Fatalf("WorkOnce error = %v, want ErrNoRuns", err)
	}
}
