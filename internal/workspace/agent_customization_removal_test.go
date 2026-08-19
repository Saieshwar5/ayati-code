package workspace

import (
	"context"
	"path/filepath"
	"testing"

	appdatabase "github.com/Saieshwar5/perpetual/internal/database"
)

func TestStoreRemovesAgentCustomizationTables(t *testing.T) {
	database, err := appdatabase.Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open database: %v", err)
	}
	defer database.Close()

	for _, table := range []string{"agents", "skills", "agent_skills", "application_settings"} {
		if _, err := database.SQL().Exec(`CREATE TABLE ` + table + ` (id TEXT PRIMARY KEY)`); err != nil {
			t.Fatalf("create legacy table %s: %v", table, err)
		}
	}
	store, err := NewStore(database)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	for _, table := range []string{"agents", "skills", "agent_skills", "application_settings"} {
		var count int
		err := store.db.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count)
		if err != nil {
			t.Fatalf("inspect table %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("legacy table %s still exists", table)
		}
	}
}
