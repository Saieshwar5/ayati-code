package workspaceruntime

import (
	"context"

	"github.com/Saieshwar5/perpetual/internal/exec"
	"github.com/Saieshwar5/perpetual/internal/vmagent"
)

// RemoteShell executes bounded commands through vmagent inside a microVM. It
// implements exec.Shell so the execution-room worker loop never knows whether
// commands run on the controller or in a Lambda MicroVM.
type RemoteShell struct {
	client *vmagent.Client
}

// NewRemoteShell builds a shell adapter for one microVM endpoint.
func NewRemoteShell(client *vmagent.Client) (*RemoteShell, error) {
	if client == nil {
		return nil, errRemoteShellClient
	}
	return &RemoteShell{client: client}, nil
}

// Execute posts one bounded command to vmagent and returns its result.
func (s *RemoteShell) Execute(ctx context.Context, request exec.ShellRequest) exec.ShellResult {
	result, err := s.client.Exec(ctx, request.Command)
	if err != nil {
		return exec.ShellResult{
			Command: request.Command, ExitCode: -1, Error: err.Error(),
		}
	}
	result.Command = request.Command
	return result
}
