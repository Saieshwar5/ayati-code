package environment

import "context"

type RuntimeSpec struct {
	Environment       Environment
	Lease             Lease
	WorkspacePath     string
	CachePath         string
	WorkspaceWritable bool
}

type Runtime struct {
	ID                string
	Name              string
	EnvironmentID     string
	WorkspaceID       string
	LeaseID           string
	Generation        int64
	ImageID           string
	Running           bool
	WorkspaceWritable bool
}

type RuntimeDriver interface {
	Create(context.Context, RuntimeSpec) (Runtime, error)
	Destroy(context.Context, RuntimeSpec, string) error
}

type StartInput struct {
	WorkspaceID            string
	PreferredEnvironmentID string
	WorkspacePath          string
	CachePath              string
	WorkspaceWritable      bool
}

type StopInput struct {
	WorkspaceID       string
	WorkspacePath     string
	CachePath         string
	WorkspaceWritable bool
}

type Assignment struct {
	Environment Environment
	Lease       Lease
	Runtime     Runtime
}
