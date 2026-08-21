package workspaceruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Saieshwar5/perpetual/internal/exec"
)

// Local is the compatibility workspace runtime: it executes commands through
// the bounded local shell on the control-plane machine. It exists so the
// workspace service can depend only on the Runtime contract while cloud-backed
// implementations are designed.
type Local struct{}

// NewLocal returns the local compatibility runtime.
func NewLocal() Runtime {
	return Local{}
}

func (Local) Start(context.Context, Ref) error {
	return nil
}

func (Local) Stop(context.Context, Ref) error {
	return nil
}

func (Local) Destroy(context.Context, Ref) error {
	return nil
}

// OpenShell creates a bounded local shell in the workspace repository
// directory. It creates the private per-workspace home before handing the
// shell to the caller, keeping host environment values out of the command.
func (Local) OpenShell(_ context.Context, ref Ref, variables map[string]string) (exec.Shell, error) {
	if strings.TrimSpace(ref.Directory) == "" {
		return nil, errors.New("workspace runtime directory is required")
	}
	if home := strings.TrimSpace(variables["HOME"]); home != "" {
		if err := os.MkdirAll(home, 0o700); err != nil {
			return nil, fmt.Errorf("create shell home: %w", err)
		}
	}
	return exec.New(variables, ref.Directory)
}
