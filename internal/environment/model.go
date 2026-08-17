package environment

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	DriverDocker = "docker"

	NetworkDisabled = "disabled"
	NetworkOutbound = "outbound"

	Provisioning         = "provisioning"
	ProvisioningReady    = "ready"
	ProvisioningFailed   = "failed"
	ProvisioningDeleting = "deleting"

	StateProvisioning = "provisioning"
	StateAvailable    = "available"
	StateOccupied     = "occupied"
	StateReleasing    = "releasing"
	StateFailed       = "failed"
	StateDeleting     = "deleting"

	LeaseAcquiring = "acquiring"
	LeaseActive    = "active"
	LeaseReleasing = "releasing"
	LeaseReleased  = "released"
	LeaseFailed    = "failed"
)

var (
	ErrNoEnvironmentAvailable = errors.New("no environment is available")
	ErrEnvironmentOccupied    = errors.New("environment is occupied")
	ErrEnvironmentQuarantined = errors.New("environment is quarantined by a failed workspace lease; delete the failed workspace first")
	ErrEnvironmentReady       = errors.New("environment is ready and does not need repair")
	ErrWorkspaceLeased        = errors.New("workspace already has an environment lease")
	ErrLeaseState             = errors.New("environment lease is in the wrong state")
)

type Environment struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Driver            string    `json:"driver"`
	ImageRef          string    `json:"image_ref"`
	ImageDigest       string    `json:"image_digest,omitempty"`
	CPUMillis         int       `json:"cpu_millis"`
	MemoryMB          int       `json:"memory_mb"`
	PIDLimit          int       `json:"pid_limit"`
	NetworkPolicy     string    `json:"network_policy"`
	ProvisioningState string    `json:"provisioning_state"`
	State             string    `json:"state"`
	Generation        int64     `json:"generation"`
	Error             string    `json:"error,omitempty"`
	Quarantined       bool      `json:"quarantined"`
	ActiveLease       *Lease    `json:"active_lease,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type CreateInput struct {
	Name          string `json:"name"`
	Driver        string `json:"driver"`
	ImageRef      string `json:"image_ref"`
	CPUMillis     int    `json:"cpu_millis"`
	MemoryMB      int    `json:"memory_mb"`
	PIDLimit      int    `json:"pid_limit"`
	NetworkPolicy string `json:"network_policy"`
}

type Lease struct {
	ID            string     `json:"id"`
	EnvironmentID string     `json:"environment_id"`
	WorkspaceID   string     `json:"workspace_id"`
	Generation    int64      `json:"generation"`
	State         string     `json:"state"`
	RuntimeID     string     `json:"runtime_id,omitempty"`
	Error         string     `json:"error,omitempty"`
	AcquiredAt    time.Time  `json:"acquired_at"`
	ActivatedAt   *time.Time `json:"activated_at,omitempty"`
	ReleasedAt    *time.Time `json:"released_at,omitempty"`
}

func normalizeCreate(input CreateInput) (CreateInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Driver = strings.TrimSpace(input.Driver)
	input.ImageRef = strings.TrimSpace(input.ImageRef)
	input.NetworkPolicy = strings.TrimSpace(input.NetworkPolicy)
	if input.Name == "" || input.ImageRef == "" {
		return CreateInput{}, errors.New("environment name and image are required")
	}
	if len(input.Name) > 80 || len(input.ImageRef) > 512 {
		return CreateInput{}, errors.New("environment name or image is too long")
	}
	if input.Driver == "" {
		input.Driver = DriverDocker
	}
	if input.Driver != DriverDocker {
		return CreateInput{}, fmt.Errorf("environment driver %q is not supported", input.Driver)
	}
	if input.CPUMillis == 0 {
		input.CPUMillis = 2000
	}
	if input.MemoryMB == 0 {
		input.MemoryMB = 4096
	}
	if input.PIDLimit == 0 {
		input.PIDLimit = 256
	}
	if input.CPUMillis < 100 || input.CPUMillis > 64000 {
		return CreateInput{}, errors.New("environment CPU must be between 100 and 64000 millicores")
	}
	if input.MemoryMB < 128 || input.MemoryMB > 262144 {
		return CreateInput{}, errors.New("environment memory must be between 128 and 262144 MB")
	}
	if input.PIDLimit < 16 || input.PIDLimit > 65535 {
		return CreateInput{}, errors.New("environment PID limit must be between 16 and 65535")
	}
	if input.NetworkPolicy == "" {
		input.NetworkPolicy = NetworkOutbound
	}
	if input.NetworkPolicy != NetworkDisabled && input.NetworkPolicy != NetworkOutbound {
		return CreateInput{}, fmt.Errorf("environment network policy %q is not supported", input.NetworkPolicy)
	}
	return input, nil
}

func stateFor(provisioning, leaseState string) string {
	switch provisioning {
	case Provisioning:
		return StateProvisioning
	case ProvisioningFailed:
		return StateFailed
	case ProvisioningDeleting:
		return StateDeleting
	}
	if leaseState == LeaseReleasing {
		return StateReleasing
	}
	if leaseState == LeaseAcquiring || leaseState == LeaseActive {
		return StateOccupied
	}
	return StateAvailable
}
