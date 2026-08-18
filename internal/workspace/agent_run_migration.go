package workspace

import (
	"context"
	"fmt"
	"time"
)

func (s *Store) migrateAgentRuns(ctx context.Context) error {
	for _, statement := range []string{
		agentRunSchema,
		`CREATE INDEX IF NOT EXISTS agent_runs_session_created ON agent_runs(session_id, created_at DESC)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS agent_runs_workspace_active ON agent_runs(workspace_id)
			WHERE status IN ('accepted', 'running')`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate agent runs: %w", err)
		}
	}
	now := formatTime(time.Now().UTC())
	if _, err := s.db.ExecContext(ctx, `UPDATE agent_runs SET status = ?,
		error = 'Agent run interrupted when Perpetual restarted', finished_at = ?, updated_at = ?
		WHERE status IN ('accepted', 'running')`, AgentRunStatusInterrupted, now, now); err != nil {
		return fmt.Errorf("recover interrupted agent runs: %w", err)
	}
	return nil
}
