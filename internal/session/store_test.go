package session

import (
	"os"
	"testing"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

func TestSessionRoundTripAndPrefixLoad(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	created, err := store.New("/workspace", "test-model")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	message := agent.Message{Role: "user", Content: "fix it"}
	if err := created.Append(message); err != nil {
		t.Fatalf("Append: %v", err)
	}
	loaded, err := store.Load(created.Info.ID[:8])
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Info.Model != "test-model" || len(loaded.Messages) != 1 ||
		loaded.Messages[0].Role != message.Role || loaded.Messages[0].Content != message.Content {
		t.Fatalf("loaded = %#v", loaded)
	}
	infos, err := store.List("/workspace")
	if err != nil || len(infos) != 1 || infos[0].ID != created.Info.ID {
		t.Fatalf("infos = %#v, error = %v", infos, err)
	}
}

func TestSessionFilesArePrivate(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	created, err := store.New("/workspace", "test-model")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	info, err := os.Stat(created.path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file permissions = %o", info.Mode().Perm())
	}
}
