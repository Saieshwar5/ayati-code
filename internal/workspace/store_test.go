package workspace

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/agent"
)

func TestStoreCreatesListsAndUpdatesWorkspace(t *testing.T) {
	database := filepath.Join(t.TempDir(), "state", "ayati.db")
	store, err := Open(database)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	info, err := os.Stat(database)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("database permissions = %v, error = %v", info.Mode().Perm(), err)
	}
	repositoryPath := filepath.Join(t.TempDir(), "repo")
	created, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "perpetual/change", CreateBranch: true, Setup: "go mod download",
		Path: repositoryPath,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != StatusCreating || created.PreparationStage != PreparationPending ||
		len(created.ConfigurationCandidates) != 0 || !created.CreateBranch || created.Authority != AuthorityExplore ||
		created.Path != filepath.Clean(repositoryPath) {
		t.Fatalf("workspace = %#v", created)
	}
	if err := store.UpdateStatus(context.Background(), created.ID, StatusReady, ""); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if err := store.UpdatePullRequest(context.Background(), created.ID, 42, "https://github.com/owner/project/pull/42"); err != nil {
		t.Fatalf("UpdatePullRequest: %v", err)
	}
	loaded, err := store.Get(context.Background(), created.ID)
	if err != nil || loaded.Status != StatusReady || loaded.PullRequestNumber != 42 {
		t.Fatalf("workspace = %#v, error = %v", loaded, err)
	}
	values, err := store.List(context.Background())
	if err != nil || len(values) != 1 || values[0].ID != created.ID {
		t.Fatalf("workspaces = %#v, error = %v", values, err)
	}
}

func TestStorePersistsCompleteAgentMessages(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sessions, err := store.ListSessions(context.Background(), value.ID)
	if err != nil || len(sessions) != 1 || sessions[0].Title != "Original session" {
		t.Fatalf("sessions = %#v, error = %v", sessions, err)
	}
	want := agent.Message{Role: "assistant", ToolCalls: []agent.ToolCall{{
		ID: "call-1", Type: "function", Function: agent.FunctionCall{Name: "shell", Arguments: `{"command":"pwd"}`},
	}}}
	if err := store.AppendMessage(context.Background(), sessions[0].ID, want); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	messages, err := store.Messages(context.Background(), sessions[0].ID)
	if err != nil || len(messages) != 1 || messages[0].ToolCalls[0].ID != "call-1" {
		t.Fatalf("messages = %#v, error = %v", messages, err)
	}
	conversation, err := store.ConversationMessages(context.Background(), sessions[0].ID)
	if err != nil || len(conversation) != 1 || conversation[0].ID < 1 || conversation[0].CreatedAt.IsZero() {
		t.Fatalf("conversation messages = %#v, error = %v", conversation, err)
	}
}

func TestStoreRejectsInvalidWorkspaceAndStatus(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.Create(context.Background(), Create{}); err == nil {
		t.Fatal("Create accepted empty workspace")
	}
	if _, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Authority: "owner", Path: t.TempDir(),
	}); err == nil {
		t.Fatal("Create accepted invalid authority")
	}
	if err := store.UpdateStatus(context.Background(), "missing", "unknown", ""); err == nil {
		t.Fatal("UpdateStatus accepted unknown status")
	}
	if _, err := store.Get(context.Background(), "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("Get error = %v", err)
	}
}
