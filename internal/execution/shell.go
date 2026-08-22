package execution

import (
	"context"

	"github.com/Saieshwar5/perpetual/internal/exec"
	"github.com/Saieshwar5/perpetual/internal/workspaceruntime"
)

// ShellRunner executes bounded commands through the workspace runtime. The
// controller keeps Git/GitHub authority; the shell only runs user/agent
// commands inside the isolated environment.
type ShellRunner interface {
	Run(ctx context.Context, command string) (exec.ShellResult, error)
}

// RuntimeShell adapts workspaceruntime.Runtime to ShellRunner.
type RuntimeShell struct {
	runtime workspaceruntime.Runtime
	ref     workspaceruntime.Ref
	env     map[string]string
}

// NewRuntimeShell returns a ShellRunner rooted at a workspace runtime.
func NewRuntimeShell(runtime workspaceruntime.Runtime, ref workspaceruntime.Ref, env map[string]string) (*RuntimeShell, error) {
	if runtime == nil {
		return nil, errNoRuntime
	}
	return &RuntimeShell{runtime: runtime, ref: ref, env: env}, nil
}

func (s *RuntimeShell) Run(ctx context.Context, command string) (exec.ShellResult, error) {
	shell, err := s.runtime.OpenShell(ctx, s.ref, s.env)
	if err != nil {
		return exec.ShellResult{}, err
	}
	return shell.Execute(ctx, exec.ShellRequest{Command: command}), nil
}
