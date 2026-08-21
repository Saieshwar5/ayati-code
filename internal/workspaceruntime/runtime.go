// Package workspaceruntime owns the boundary between the Perpetual control
// plane and the isolated execution environment that runs workspace commands.
// The control plane never executes workspace setup, inspection, or agent
// commands itself; it asks a WorkspaceRuntime to do so. The local adapter is
// the compatibility implementation while cloud-backed runtimes are designed.
package workspaceruntime

import (
	"context"

	"github.com/Saieshwar5/perpetual/internal/exec"
)

// Ref identifies one workspace runtime instance. Directory is the repository
// root the runtime exposes, CacheDir is the persistent tool-cache root, and
// RuntimeID is the provider-specific instance reference when one exists.
type Ref struct {
	ID        string
	RuntimeID string
	Directory string
	CacheDir  string
}

// RuntimeState is a durable runtime lifecycle state recorded on the workspace.
// It is independent from workspace status: a ready workspace may have a
// stopped runtime, and a creating workspace may have a running runtime.
const (
	RuntimeStateNotCreated = "not_created"
	RuntimeStateCreating   = "creating"
	RuntimeStateRunning    = "running"
	RuntimeStateStopped    = "stopped"
	RuntimeStateDestroying = "destroying"
	RuntimeStateFailed     = "failed"
)

// Runtime is the contract workspace lifecycle, preparation, review, publish,
// and the planned agent use to interact with a workspace's isolated runtime.
type Runtime interface {
	// Start brings an existing workspace runtime to an active state. It is
	// idempotent: starting an already active runtime must succeed.
	Start(context.Context, Ref) error
	// Stop brings an active workspace runtime back to a saved state. It is
	// idempotent: stopping an already stopped runtime must succeed.
	Stop(context.Context, Ref) error
	// OpenShell returns a bounded shell whose working directory is the
	// workspace repository directory inside the runtime. The provided
	// variables are the only environment values the shell may inherit.
	OpenShell(context.Context, Ref, map[string]string) (exec.Shell, error)
	// Destroy releases all runtime-owned resources without touching durable
	// control-plane state such as Git branches, pull requests, or SQLite.
	Destroy(context.Context, Ref) error
}
