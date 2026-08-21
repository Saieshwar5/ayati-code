package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Saieshwar5/perpetual/internal/workspaceruntime"
)

var runtimeStates = map[string]bool{
	workspaceruntime.RuntimeStateNotCreated: true,
	workspaceruntime.RuntimeStateCreating:   true,
	workspaceruntime.RuntimeStateRunning:    true,
	workspaceruntime.RuntimeStateStopped:    true,
	workspaceruntime.RuntimeStateDestroying: true,
	workspaceruntime.RuntimeStateFailed:     true,
}

// UpdateRuntimeState durably records the workspace runtime lifecycle state.
func (s *Store) UpdateRuntimeState(ctx context.Context, id, state string) error {
	state = strings.TrimSpace(state)
	if !runtimeStates[state] {
		return fmt.Errorf("invalid runtime state %q", state)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE workspaces SET
		runtime_state = ?, runtime_updated_at = ? WHERE id = ?`,
		state, formatTime(time.Now().UTC()), strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("update workspace runtime state: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return errors.New("workspace not found")
	}
	return nil
}
