package workspace

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/agent"
)

func TestPerpetualIdentityMigrationRenamesBuiltInAgent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "perpetual.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE agents SET name = 'Ayati' WHERE id = ?`, agent.BuiltinAgentID); err != nil {
		t.Fatalf("seed legacy identity: %v", err)
	}
	if _, err := store.db.Exec(`PRAGMA user_version = 8`); err != nil {
		t.Fatalf("seed legacy schema version: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	definition, err := store.GetAgent(context.Background(), agent.BuiltinAgentID)
	if err != nil {
		t.Fatalf("Agent: %v", err)
	}
	if definition.Name != "Perpetual" {
		t.Fatalf("built-in agent name = %q", definition.Name)
	}
}
