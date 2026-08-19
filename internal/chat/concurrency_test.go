package chat

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Saieshwar5/perpetual/internal/agent"
	"github.com/Saieshwar5/perpetual/internal/workspace"
)

type concurrentProvider struct {
	started chan struct{}
	release chan struct{}
}

func (p *concurrentProvider) Next(ctx context.Context, _ agent.Request) (agent.Message, error) {
	p.started <- struct{}{}
	select {
	case <-p.release:
		return agent.Message{Role: "assistant", Content: "done"}, nil
	case <-ctx.Done():
		return agent.Message{}, ctx.Err()
	}
}

type mappedWorkspaceRuntime struct {
	values map[string]workspace.Workspace
}

func (r mappedWorkspaceRuntime) Shell(_ context.Context, workspaceID string) (
	agent.Shell,
	workspace.Workspace,
	error,
) {
	value, ok := r.values[workspaceID]
	if !ok {
		return nil, workspace.Workspace{}, fmt.Errorf("workspace %s is unavailable", workspaceID)
	}
	return &fakeShell{}, value, nil
}

func TestServiceRunsDifferentWorkspacesConcurrently(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	values := make(map[string]workspace.Workspace, 2)
	sessionIDs := make(map[string]string, 2)
	for index := 1; index <= 2; index++ {
		value, createErr := store.Create(context.Background(), workspace.Create{
			Repository: fmt.Sprintf("owner/project-%d", index),
			CloneURL:   fmt.Sprintf("https://github.com/owner/project-%d.git", index),
			BaseBranch: "main",
			Branch:     fmt.Sprintf("perpetual/change-%d", index),
			Path:       filepath.Join(t.TempDir(), fmt.Sprintf("repo-%d", index)),
		})
		if createErr != nil {
			t.Fatalf("Create workspace %d: %v", index, createErr)
		}
		if updateErr := store.UpdateStatus(context.Background(), value.ID, workspace.StatusReady, ""); updateErr != nil {
			t.Fatalf("UpdateStatus workspace %d: %v", index, updateErr)
		}
		sessions, listErr := store.ListSessions(context.Background(), value.ID)
		if listErr != nil || len(sessions) != 1 {
			t.Fatalf("sessions = %#v, error = %v", sessions, listErr)
		}
		values[value.ID] = value
		sessionIDs[value.ID] = sessions[0].ID
	}

	provider := &concurrentProvider{started: make(chan struct{}, 2), release: make(chan struct{})}
	service, err := New(store, mappedWorkspaceRuntime{values: values}, provider, "test-model")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	appContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	runs := make([]workspace.AgentRun, 0, 2)
	for workspaceID, sessionID := range sessionIDs {
		run, startErr := service.Start(appContext, workspaceID, sessionID, "work on it")
		if startErr != nil {
			t.Fatalf("Start workspace %s: %v", workspaceID, startErr)
		}
		runs = append(runs, run)
	}
	for range 2 {
		select {
		case <-provider.started:
		case <-time.After(time.Second):
			t.Fatal("different workspaces did not reach the provider concurrently")
		}
	}
	close(provider.release)
	waitForCompletedRuns(t, store, runs)
}

func waitForCompletedRuns(t *testing.T, store *workspace.Store, runs []workspace.AgentRun) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		completed := 0
		for _, run := range runs {
			loaded, err := store.AgentRun(context.Background(), run.WorkspaceID, run.SessionID, run.ID)
			if err == nil && loaded.Status == workspace.AgentRunStatusCompleted {
				completed++
			}
		}
		if completed == len(runs) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("concurrent workspace runs did not complete")
}
