package workspace

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/agent"
	_ "modernc.org/sqlite"
)

func TestStoreKeepsSessionConversationsSeparate(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value := createTestWorkspace(t, store)
	sessions, err := store.ListSessions(context.Background(), value.ID)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("initial sessions = %#v, error = %v", sessions, err)
	}
	second, err := store.CreateSession(context.Background(), value.ID, "More changes")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := store.AppendMessage(context.Background(), sessions[0].ID,
		agent.Message{Role: "user", Content: "first"}); err != nil {
		t.Fatalf("AppendMessage first: %v", err)
	}
	if err := store.AppendMessage(context.Background(), second.ID,
		agent.Message{Role: "user", Content: "second"}); err != nil {
		t.Fatalf("AppendMessage second: %v", err)
	}
	firstMessages, _ := store.Messages(context.Background(), sessions[0].ID)
	secondMessages, _ := store.Messages(context.Background(), second.ID)
	if len(firstMessages) != 1 || firstMessages[0].Content != "first" ||
		len(secondMessages) != 1 || secondMessages[0].Content != "second" {
		t.Fatalf("first = %#v, second = %#v", firstMessages, secondMessages)
	}
	if _, err := store.GetSession(context.Background(), "another-workspace", second.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-workspace GetSession error = %v", err)
	}
	if err := store.DeleteSession(context.Background(), value.ID, second.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if err := store.DeleteSession(context.Background(), value.ID, sessions[0].ID); err == nil {
		t.Fatal("DeleteSession removed the last workspace session")
	}
}

func TestStoreMigratesWorkspaceMessagesToOriginalSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	now := formatTime(time.Now().UTC())
	for _, statement := range []string{
		workspaceSchema,
		`CREATE TABLE messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			payload TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatalf("create legacy schema: %v", err)
		}
	}
	_, err = database.Exec(`INSERT INTO workspaces (
		id, repository, clone_url, base_branch, branch, create_branch, setup_command, path,
		sandbox_name, status, error, pull_request_number, pull_request_url, created_at, updated_at
	) VALUES ('workspace-1', 'owner/project', 'https://github.com/owner/project.git', 'main',
		'ayati/change', 0, '', '/tmp/legacy-repo', 'legacy-sandbox', 'review', '', 0, '', ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert legacy workspace: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO messages (workspace_id, payload, created_at)
		VALUES ('workspace-1', '{"role":"user","content":"legacy message"}', ?)`, now); err != nil {
		t.Fatalf("insert legacy message: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sessions, err := store.ListSessions(context.Background(), "workspace-1")
	if err != nil || len(sessions) != 1 || sessions[0].Title != "Original session" ||
		sessions[0].Status != SessionStatusReview {
		t.Fatalf("migrated sessions = %#v, error = %v", sessions, err)
	}
	messages, err := store.Messages(context.Background(), sessions[0].ID)
	if err != nil || len(messages) != 1 || messages[0].Content != "legacy message" {
		t.Fatalf("migrated messages = %#v, error = %v", messages, err)
	}
	workspace, err := store.Get(context.Background(), "workspace-1")
	if err != nil || workspace.Status != StatusReady {
		t.Fatalf("migrated workspace = %#v, error = %v", workspace, err)
	}
}

func TestStoreTitlesNewSessionFromFirstMessage(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value := createTestWorkspace(t, store)
	session, err := store.CreateSession(context.Background(), value.ID, "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := store.TitleSessionFromMessage(context.Background(), session.ID,
		"Work on it.\n\nImprove the workspace session navigation"); err != nil {
		t.Fatalf("TitleSessionFromMessage: %v", err)
	}
	loaded, err := store.GetSession(context.Background(), value.ID, session.ID)
	if err != nil || loaded.Title != "Improve the workspace session navigation" {
		t.Fatalf("session = %#v, error = %v", loaded, err)
	}
}

func createTestWorkspace(t *testing.T, store *Store) Workspace {
	t.Helper()
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "ayati/change", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	return value
}
