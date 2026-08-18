package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestStorePersistsAgentRunLifecycle(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "perpetual/run", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sessions, err := store.ListSessions(context.Background(), value.ID)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %#v, error = %v", sessions, err)
	}
	run, err := store.BeginAgentRun(context.Background(), value.ID, sessions[0].ID, "inspect this")
	if err != nil {
		t.Fatalf("BeginAgentRun: %v", err)
	}
	if _, err := store.BeginAgentRun(context.Background(), value.ID, sessions[0].ID, "again"); !errors.Is(err, ErrAgentRunActive) {
		t.Fatalf("second BeginAgentRun error = %v", err)
	}
	loadedSession, err := store.GetSession(context.Background(), value.ID, sessions[0].ID)
	if err != nil || loadedSession.Status != SessionStatusWorking || loadedSession.ActiveRunID != run.ID {
		t.Fatalf("active session = %#v, error = %v", loadedSession, err)
	}
	messages, err := store.Messages(context.Background(), sessions[0].ID)
	if err != nil || len(messages) != 1 || messages[0].Content != "inspect this" {
		t.Fatalf("messages = %#v, error = %v", messages, err)
	}
	if err := store.MarkAgentRunRunning(context.Background(), run.ID); err != nil {
		t.Fatalf("MarkAgentRunRunning: %v", err)
	}
	if err := store.FinishAgentRun(context.Background(), run.ID, AgentRunStatusCompleted,
		SessionStatusIdle, ""); err != nil {
		t.Fatalf("FinishAgentRun: %v", err)
	}
	loadedRun, err := store.AgentRun(context.Background(), value.ID, sessions[0].ID, run.ID)
	if err != nil || loadedRun.Status != AgentRunStatusCompleted || loadedRun.StartedAt == nil ||
		loadedRun.FinishedAt == nil {
		t.Fatalf("completed run = %#v, error = %v", loadedRun, err)
	}
	loadedSession, _ = store.GetSession(context.Background(), value.ID, sessions[0].ID)
	if loadedSession.Status != SessionStatusIdle || loadedSession.ActiveRunID != "" {
		t.Fatalf("completed session = %#v", loadedSession)
	}
}

func TestStoreRecoversInterruptedAgentRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ayati.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sessions, _ := store.ListSessions(context.Background(), value.ID)
	run, err := store.BeginAgentRun(context.Background(), value.ID, sessions[0].ID, "long work")
	if err != nil {
		t.Fatalf("BeginAgentRun: %v", err)
	}
	if err := store.MarkAgentRunRunning(context.Background(), run.ID); err != nil {
		t.Fatalf("MarkAgentRunRunning: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	loadedRun, err := store.AgentRun(context.Background(), value.ID, sessions[0].ID, run.ID)
	if err != nil || loadedRun.Status != AgentRunStatusInterrupted || loadedRun.FinishedAt == nil {
		t.Fatalf("recovered run = %#v, error = %v", loadedRun, err)
	}
	loadedSession, err := store.GetSession(context.Background(), value.ID, sessions[0].ID)
	if err != nil || loadedSession.Status != SessionStatusFailed || loadedSession.ActiveRunID != "" {
		t.Fatalf("recovered session = %#v, error = %v", loadedSession, err)
	}
}
