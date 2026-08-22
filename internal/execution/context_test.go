package execution

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/workspace"
)

func TestShouldCompactThreshold(t *testing.T) {
	settings := DefaultSettings()
	if ShouldCompact(100_000, 200_000, settings) {
		t.Fatal("expected no compaction under the threshold")
	}
	if !ShouldCompact(185_000, 200_000, settings) {
		t.Fatal("expected compaction near the window edge")
	}
	if ShouldCompact(185_000, 200_000, Settings{Enabled: false}) {
		t.Fatal("expected disabled compaction to never trigger")
	}
}

func TestCutStepsKeepsShellWithModelTurn(t *testing.T) {
	steps := []workspace.RunStep{
		{StepKey: "step-0", Kind: workspace.StepModel, Output: map[string]any{"content": strings.Repeat("a", 2000)}},
		{StepKey: "shell-0", Kind: workspace.StepShell, Input: map[string]any{"command": "echo old"}, Output: map[string]any{"stdout": "old"}},
		{StepKey: "step-1", Kind: workspace.StepModel, Output: map[string]any{"content": strings.Repeat("b", 2000)}},
		{StepKey: "shell-1", Kind: workspace.StepShell, Input: map[string]any{"command": "echo new"}, Output: map[string]any{"stdout": "new"}},
	}
	kept, summarized := CutSteps(steps, 100)
	if len(kept) == 0 || len(summarized) == 0 {
		t.Fatalf("kept=%d summarized=%d", len(kept), len(summarized))
	}
	// A shell step may live in either side, but must always follow its model step.
	for i, side := range [][]workspace.RunStep{kept, summarized} {
		for j := range side {
			if side[j].Kind == workspace.StepShell && j == 0 {
				t.Fatalf("side %d starts with a shell step without its model turn", i)
			}
		}
	}
}

func TestCompactorGeneratesStructuredSummary(t *testing.T) {
	provider := &fakeProvider{responses: []ModelResponse{
		{StopReason: "stop", Content: "## Goal\nFix tests\n## Progress\n- [x] added test\n"},
	}}
	compactor, err := NewCompactor(provider, DefaultSettings())
	if err != nil {
		t.Fatalf("NewCompactor: %v", err)
	}
	summary, err := compactor.Compact(context.Background(), []workspace.RunStep{
		{StepKey: "step-0", Kind: workspace.StepModel, Output: map[string]any{"content": "work"}},
	}, "")
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if !strings.Contains(summary, "## Goal") || !strings.Contains(summary, "Fix tests") {
		t.Fatalf("summary = %q", summary)
	}
}

func TestBuildContextIncludesMemoryAndSteps(t *testing.T) {
	store, err := workspace.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	ws, err := store.Create(ctx, workspace.Create{
		UserID: "user-1", Repository: "owner/ctx", CloneURL: "https://github.com/owner/ctx.git",
		BaseBranch: "main", Branch: "main",
		Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	session, err := store.CreateSession(ctx, ws.ID, "ctx session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	run, err := store.EnqueueRun(ctx, workspace.EnqueueRunInput{
		UserID: "user-1", WorkspaceID: ws.ID, SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("EnqueueRun: %v", err)
	}
	if err := store.AppendRunStep(ctx, run.ID, "step-0", workspace.StepShell, "done",
		map[string]any{"command": "echo hi"}, map[string]any{"stdout": "hi", "exit_code": float64(0)}); err != nil {
		t.Fatalf("AppendRunStep: %v", err)
	}
	built, err := BuildContext(ctx, store, run)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	joined := strings.Join(built.Messages, "\n")
	if !strings.Contains(joined, "owner/ctx") || !strings.Contains(joined, "echo hi") {
		t.Fatalf("context messages = %q", joined)
	}
	if built.TokenCount() <= 0 {
		t.Fatal("expected positive token count")
	}
}
