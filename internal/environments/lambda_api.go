package environments

import (
	"context"
)

// RunMicrovmInput is the control-plane subset Perpetual uses to launch an
// instance from an existing image.
type RunMicrovmInput struct {
	ImageARN         string
	ImageVersion     string
	ExecutionRoleARN string
}

// Instance is one running/suspended Lambda microVM.
type Instance struct {
	MicrovmID string
	Endpoint  string
	State     string
	ImageARN  string
}

// ImageRef identifies a built microVM image version.
type ImageRef struct {
	ImageARN string
	Version  string
	State    string
}

// API abstracts the Lambda MicroVMs control plane. The AWS adapter uses the
// Go SDK; tests use a fake implementation.
type API interface {
	CreateMicrovmImage(context.Context) (ImageRef, error)
	GetMicrovmImage(context.Context) (ImageRef, error)
	RunMicrovm(context.Context, RunMicrovmInput) (Instance, error)
	AuthToken(context.Context, string) (string, error)
	SuspendMicrovm(context.Context, string) error
	ResumeMicrovm(context.Context, string) error
	TerminateMicrovm(context.Context, string) error
	GetMicrovm(context.Context, string) (Instance, error)
}
