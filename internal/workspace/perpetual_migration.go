package workspace

import (
	"context"
	"fmt"
	"time"

	"github.com/Saieshwar5/perpetual/internal/agent"
)

func (s *Store) migratePerpetualIdentity(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE agents
		SET name = 'Perpetual', updated_at = ?
		WHERE id = ? AND built_in = 1 AND name = 'Ayati'`,
		formatTime(time.Now().UTC()), agent.BuiltinAgentID); err != nil {
		return fmt.Errorf("rename built-in agent: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA user_version = 9`); err != nil {
		return fmt.Errorf("record Perpetual identity migration: %w", err)
	}
	return nil
}
