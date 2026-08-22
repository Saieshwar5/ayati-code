package workspace

import (
	"context"
	"fmt"
)

// migrateRemoveAgentRuns drops the removed agent execution table and advances
// the schema version. Sessions and their stored messages are preserved; the
// The current execution-room model reintroduces agent_runs with durable step.
func (s *Store) migrateRemoveAgentRuns(ctx context.Context) error {
	if _, err := s.execContext(ctx, `DROP TABLE IF EXISTS agent_runs`); err != nil {
		return fmt.Errorf("remove agent runs table: %w", err)
	}
	if err := s.setSchemaVersion(ctx, 12); err != nil {
		return fmt.Errorf("record agent runs removal: %w", err)
	}
	return nil
}
