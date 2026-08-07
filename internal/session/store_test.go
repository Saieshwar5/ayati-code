package session

import (
	"path/filepath"
	"testing"

	"github.com/sai-eshwar/no-nonsense-coding-ai/internal/chat"
)

func TestCreateAppendOpenAndContinue(t *testing.T) {
	root := t.TempDir()
	store := Store{Dir: filepath.Join(root, "sessions")}
	created, err := store.Create(root)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	message := chat.Message{Role: "user", Content: "hello"}
	if err := store.Append(created, message); err != nil {
		t.Fatalf("Append: %v", err)
	}

	opened, err := store.Open(created.Header.ID[:6])
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(opened.Messages) != 1 || opened.Messages[0].Content != "hello" {
		t.Fatalf("unexpected restored messages: %+v", opened.Messages)
	}

	continued, err := store.ContinueRecent(root)
	if err != nil {
		t.Fatalf("ContinueRecent: %v", err)
	}
	if continued.Header.ID != created.Header.ID {
		t.Fatalf("continued %s, want %s", continued.Header.ID, created.Header.ID)
	}
}

func TestContinueRecentIsScopedToWorkingDirectory(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "sessions")}
	first, err := store.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	secondCWD := t.TempDir()
	second, err := store.ContinueRecent(secondCWD)
	if err != nil {
		t.Fatal(err)
	}
	if second.Header.ID == first.Header.ID {
		t.Fatal("continued a session belonging to another working directory")
	}
}

func TestSummaryCheckpointPersistsAndReloads(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "sessions")}
	current, err := store.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []chat.Message{
		{Role: "user", Content: "build it"},
		{Role: "assistant", Content: "working"},
		{Role: "user", Content: "continue"},
	} {
		if err := store.Append(current, message); err != nil {
			t.Fatal(err)
		}
	}
	checkpoint := Summary{Content: "Built the initial project. Continue the latest task.", CoveredMessages: 2}
	if err := store.AppendSummary(current, checkpoint); err != nil {
		t.Fatalf("AppendSummary: %v", err)
	}
	reloaded, err := store.Open(current.Header.ID)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if reloaded.Summary == nil || reloaded.Summary.Content != checkpoint.Content || reloaded.Summary.CoveredMessages != 2 {
		t.Fatalf("unexpected summary: %#v", reloaded.Summary)
	}
	if len(reloaded.Messages) != 3 {
		t.Fatalf("summary changed exact message history: %d", len(reloaded.Messages))
	}
}
