package environment

import (
	"context"
	"errors"
)

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

type ReplaceInput struct {
	WorkspaceID               string
	WorkspacePath             string
	CachePath                 string
	PreviousWorkspaceWritable bool
	WorkspaceWritable         bool
}

type Assignment struct {
	Environment Environment
	Lease       Lease
	Runtime     Runtime
}

type ReplacementError struct {
	Cause     error
	Recovered bool
}

func (e *ReplacementError) Error() string { return e.Cause.Error() }

func (e *ReplacementError) Unwrap() error { return e.Cause }

func ReplacementRecovered(err error) bool {
	var replacement *ReplacementError
	return errors.As(err, &replacement) && replacement.Recovered
}
