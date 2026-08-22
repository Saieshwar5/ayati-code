package workspace

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestRunLifecycleOnSQLite(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	workspace, err := store.Create(context.Background(), Create{
		UserID: "user-1", Repository: "owner/run-project",
		CloneURL:   "https://github.com/owner/run-project.git",
		BaseBranch: "main", Branch: "main",
		Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	session, err := store.CreateSession(context.Background(), workspace.ID, "run session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	run, err := store.EnqueueRun(context.Background(), EnqueueRunInput{
		UserID: "user-1", WorkspaceID: workspace.ID, SessionID: session.ID,
		MaxSteps: 50, Deadline: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("EnqueueRun: %v", err)
	}
	if run.State != RunQueued {
		t.Fatalf("state = %q", run.State)
	}

	got, err := store.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.SessionID != session.ID {
		t.Fatalf("session = %q", got.SessionID)
	}

	claimed, err := store.ClaimNextRun(context.Background())
	if err != nil {
		t.Fatalf("ClaimNextRun: %v", err)
	}
	if claimed.ID != run.ID || claimed.State != RunRunning {
		t.Fatalf("claimed = %#v", claimed)
	}

	if err := store.AppendRunStep(context.Background(), run.ID, "step-1", StepShell, "done",
		map[string]any{"command": "echo hi"},
		map[string]any{"stdout": "hi\n", "exit_code": 0}); err != nil {
		t.Fatalf("AppendRunStep: %v", err)
	}
	steps, err := store.RunSteps(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("RunSteps: %v", err)
	}
	if len(steps) != 1 || steps[0].StepKey != "step-1" || steps[0].Output["stdout"] != "hi\n" {
		t.Fatalf("steps = %#v", steps)
	}

	if err := store.SaveWorkMemory(context.Background(), run.ID,
		map[string]any{"goal": "fix tests"}, 0); err != nil {
		t.Fatalf("SaveWorkMemory: %v", err)
	}
	memory, err := store.WorkMemory(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("WorkMemory: %v", err)
	}
	if memory.Notes["goal"] != "fix tests" || memory.Version != 1 {
		t.Fatalf("memory = %#v", memory)
	}

	if err := store.TouchRunLease(context.Background(), run.ID); err != nil {
		t.Fatalf("TouchRunLease: %v", err)
	}
	if err := store.CompleteRun(context.Background(), run.ID, "done"); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	completed, err := store.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun completed: %v", err)
	}
	if completed.State != RunCompleted {
		t.Fatalf("completed state = %q", completed.State)
	}
}

func TestAppendRunStepIsIdempotent(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	workspace, err := store.Create(context.Background(), Create{
		Repository: "owner/idem", CloneURL: "https://github.com/owner/idem.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	session, err := store.CreateSession(context.Background(), workspace.ID, "idem")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	run, err := store.EnqueueRun(context.Background(), EnqueueRunInput{
		UserID: "user-1", WorkspaceID: workspace.ID, SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("EnqueueRun: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := store.AppendRunStep(context.Background(), run.ID, "same-key", StepModel, "done",
			map[string]any{"i": i}, map[string]any{"i": i}); err != nil {
			t.Fatalf("AppendRunStep %d: %v", i, err)
		}
	}
	steps, err := store.RunSteps(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("RunSteps: %v", err)
	}
	if len(steps) != 1 || steps[0].Output["i"] != float64(1) {
		t.Fatalf("expected one updated step, got %#v", steps)
	}
}
