package workspace

import (
	"testing"

	appdatabase "github.com/Saieshwar5/perpetual/internal/database"
)

func TestQConvertsPlaceholdersForPostgres(t *testing.T) {
	store := &Store{dialect: appdatabase.ProviderPostgres}
	got := store.q("SELECT * FROM workspaces WHERE id = ? AND user_id = ?")
	want := "SELECT * FROM workspaces WHERE id = $1 AND user_id = $2"
	if got != want {
		t.Fatalf("q() = %q, want %q", got, want)
	}
}

func TestQLeavesSQLiteQueriesUnchanged(t *testing.T) {
	store := &Store{dialect: appdatabase.ProviderSQLite}
	query := "SELECT * FROM workspaces WHERE id = ?"
	if got := store.q(query); got != query {
		t.Fatalf("q() = %q, want %q", got, query)
	}
}
