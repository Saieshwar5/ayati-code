package workspace

import (
	"context"
	"fmt"
)

// migrateRemoveAgentRuns drops the removed agent execution table and advances
// the schema version. Sessions and their stored messages are preserved; the
// new agent will define its own execution model.
func (s *Store) migrateRemoveAgentRuns(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `DROP TABLE IF EXISTS agent_runs`); err != nil {
		return fmt.Errorf("remove agent runs table: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA user_version = 12`); err != nil {
		return fmt.Errorf("record agent runs removal: %w", err)
	}
	return nil
}
