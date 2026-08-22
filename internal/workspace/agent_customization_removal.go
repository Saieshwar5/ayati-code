package workspace

import (
	"context"
	"fmt"
)

func (s *Store) removeAgentCustomization(ctx context.Context) error {
	for _, table := range []string{"agent_skills", "skills", "application_settings", "agents"} {
		if _, err := s.execContext(ctx, `DROP TABLE IF EXISTS `+table); err != nil {
			return fmt.Errorf("remove agent customization table %s: %w", table, err)
		}
	}
	return nil
}
