package workspace

import (
	"context"

	"github.com/Saieshwar5/perpetual/internal/exec"
	"github.com/Saieshwar5/perpetual/internal/workspaceruntime"
)

// fakeRuntime records runtime openings and lifecycle calls so tests can assert
// which workspace variables, directories, and transitions a workspace receives
// without executing on the host.
type fakeRuntime struct {
	shell      exec.Shell
	variables  []map[string]string
	dirs       []string
	started    []workspaceruntime.Ref
	stopped    []workspaceruntime.Ref
	destroyed  []workspaceruntime.Ref
	openErr    error
	startErr   error
	stopErr    error
	destroyErr error
}

func (f *fakeRuntime) OpenShell(_ context.Context, ref workspaceruntime.Ref, variables map[string]string) (exec.Shell, error) {
	f.variables = append(f.variables, variables)
	f.dirs = append(f.dirs, ref.Directory)
	return f.shell, f.openErr
}

func (f *fakeRuntime) Start(_ context.Context, ref workspaceruntime.Ref) error {
	f.started = append(f.started, ref)
	return f.startErr
}

func (f *fakeRuntime) Stop(_ context.Context, ref workspaceruntime.Ref) error {
	f.stopped = append(f.stopped, ref)
	return f.stopErr
}

func (f *fakeRuntime) Destroy(_ context.Context, ref workspaceruntime.Ref) error {
	f.destroyed = append(f.destroyed, ref)
	return f.destroyErr
}
