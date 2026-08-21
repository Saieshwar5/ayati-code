package workspaceruntime

import (
	"context"
	"errors"
	"strings"

	"github.com/Saieshwar5/perpetual/internal/exec"
)

// CloudConfig carries the minimum configuration a cloud-backed runtime needs
// to be selected. The concrete provider implementation arrives in a later
// branch; this stub proves the selection and failure path.
type CloudConfig struct {
	Endpoint string
	Token    string
	Pool     string
}

// Cloud is the cloud-runtime seam. It is intentionally a stub: lifecycle and
// shell calls fail with a clear error, and NewCloud fails when required
// configuration is missing.
type Cloud struct{ config CloudConfig }

// NewCloud validates cloud runtime configuration and returns the seam.
func NewCloud(config CloudConfig) (Runtime, error) {
	if strings.TrimSpace(config.Endpoint) == "" || strings.TrimSpace(config.Token) == "" {
		return nil, errors.New("cloud runtime is not configured")
	}
	return Cloud{config: config}, nil
}

func (Cloud) Start(context.Context, Ref) error {
	return errors.New("cloud runtime is not implemented")
}

func (Cloud) Stop(context.Context, Ref) error {
	return errors.New("cloud runtime is not implemented")
}

func (Cloud) Destroy(context.Context, Ref) error {
	return errors.New("cloud runtime is not implemented")
}

func (Cloud) OpenShell(context.Context, Ref, map[string]string) (exec.Shell, error) {
	return nil, errors.New("cloud runtime shell is not implemented")
}
