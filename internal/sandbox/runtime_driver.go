package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/environment"
)

const (
	runtimeNamePrefix = "ayati-runtime-"
	labelManaged      = "ayati.runtime"
	labelEnvironment  = "ayati.environment"
	labelWorkspace    = "ayati.workspace"
	labelLease        = "ayati.lease"
	labelGeneration   = "ayati.generation"
)

type DockerDriver struct {
	runner runner
}

func NewDockerDriver() (*DockerDriver, error) {
	docker, err := exec.LookPath("docker")
	if err != nil {
		return nil, fmt.Errorf("find docker: %w", err)
	}
	return &DockerDriver{runner: osRunner{docker: docker}}, nil
}

func (d *DockerDriver) Create(
	ctx context.Context, spec environment.RuntimeSpec,
) (environment.Runtime, error) {
	spec, err := normalizeRuntimeSpec(spec)
	if err != nil {
		return environment.Runtime{}, err
	}
	if spec.Environment.ProvisioningState != environment.ProvisioningReady ||
		spec.Lease.State != environment.LeaseAcquiring {
		return environment.Runtime{}, errors.New("runtime requires a ready environment and acquiring lease")
	}
	name := runtimeName(spec)
	runtime, exists, err := d.inspect(ctx, spec, name)
	if err != nil {
		return environment.Runtime{}, err
	}
	if exists {
		if runtime.Running {
			return runtime, nil
		}
		if _, err := d.run(ctx, "start", runtime.ID); err != nil {
			return environment.Runtime{}, fmt.Errorf("start existing environment runtime: %w", err)
		}
		return d.requireRuntime(ctx, spec, runtime.ID)
	}
	if err := os.MkdirAll(spec.WorkspacePath, 0o700); err != nil {
		return environment.Runtime{}, fmt.Errorf("create workspace directory: %w", err)
	}
	if err := os.MkdirAll(spec.CachePath, 0o700); err != nil {
		return environment.Runtime{}, fmt.Errorf("create workspace cache: %w", err)
	}
	result, err := d.run(ctx, runtimeCreateArguments(spec, name)...)
	if err != nil {
		return environment.Runtime{}, fmt.Errorf("create environment runtime: %w", err)
	}
	runtimeID := strings.TrimSpace(result.stdout)
	if !validDockerID(runtimeID) {
		return environment.Runtime{}, d.cleanupFailure(name,
			errors.New("create environment runtime: Docker returned an invalid identity"))
	}
	if _, err := d.run(ctx, "start", runtimeID); err != nil {
		return environment.Runtime{}, d.cleanupFailure(runtimeID,
			fmt.Errorf("start environment runtime: %w", err))
	}
	runtime, err = d.requireRuntime(ctx, spec, runtimeID)
	if err != nil {
		return environment.Runtime{}, d.cleanupFailure(runtimeID, err)
	}
	return runtime, nil
}

func (d *DockerDriver) Inspect(
	ctx context.Context, spec environment.RuntimeSpec, runtimeID string,
) (environment.Runtime, bool, error) {
	spec, err := normalizeRuntimeSpec(spec)
	if err != nil {
		return environment.Runtime{}, false, err
	}
	target, err := runtimeTarget(spec, runtimeID)
	if err != nil {
		return environment.Runtime{}, false, err
	}
	return d.inspect(ctx, spec, target)
}

func (d *DockerDriver) Destroy(
	ctx context.Context, spec environment.RuntimeSpec, runtimeID string,
) error {
	spec, err := normalizeRuntimeSpec(spec)
	if err != nil {
		return err
	}
	target, err := runtimeTarget(spec, runtimeID)
	if err != nil {
		return err
	}
	runtime, exists, err := d.inspect(ctx, spec, target)
	if err != nil || !exists {
		return err
	}
	if _, err := d.run(ctx, "rm", "--force", "--volumes", runtime.ID); err != nil {
		return fmt.Errorf("remove environment runtime: %w", err)
	}
	if _, exists, err := d.inspect(ctx, spec, runtimeName(spec)); err != nil {
		return fmt.Errorf("verify environment runtime removal: %w", err)
	} else if exists {
		return errors.New("verify environment runtime removal: container still exists")
	}
	return nil
}

func (d *DockerDriver) requireRuntime(
	ctx context.Context, spec environment.RuntimeSpec, target string,
) (environment.Runtime, error) {
	runtime, exists, err := d.inspect(ctx, spec, target)
	if err != nil {
		return environment.Runtime{}, fmt.Errorf("verify environment runtime: %w", err)
	}
	if !exists {
		return environment.Runtime{}, errors.New("verify environment runtime: container is missing")
	}
	if !runtime.Running {
		return environment.Runtime{}, errors.New("verify environment runtime: container is not running")
	}
	return runtime, nil
}

func (d *DockerDriver) cleanupFailure(runtimeID string, cause error) error {
	if !validDockerID(runtimeID) && !validRuntimeName(runtimeID) {
		return cause
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := d.run(ctx, "rm", "--force", "--volumes", runtimeID); err != nil {
		return errors.Join(cause, fmt.Errorf("clean failed environment runtime: %w", err))
	}
	return cause
}

func (d *DockerDriver) run(ctx context.Context, arguments ...string) (commandResult, error) {
	result, err := d.runner.Run(ctx, arguments...)
	if err == nil {
		return result, nil
	}
	message := strings.TrimSpace(result.stderr)
	if message == "" {
		message = err.Error()
	}
	return result, errors.New(message)
}

func runtimeCreateArguments(spec environment.RuntimeSpec, name string) []string {
	workspaceMount := "type=bind,src=" + spec.WorkspacePath + ",dst=/workspace"
	if !spec.WorkspaceWritable {
		workspaceMount += ",readonly"
	}
	network := "bridge"
	if spec.Environment.NetworkPolicy == environment.NetworkDisabled {
		network = "none"
	}
	labels := runtimeLabels(spec)
	return []string{
		"create", "--name", name,
		"--label", labelManaged + "=true",
		"--label", labelEnvironment + "=" + labels[labelEnvironment],
		"--label", labelWorkspace + "=" + labels[labelWorkspace],
		"--label", labelLease + "=" + labels[labelLease],
		"--label", labelGeneration + "=" + labels[labelGeneration],
		"--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--restart", "no", "--user", "1000:1000", "--network", network,
		"--pids-limit", strconv.Itoa(spec.Environment.PIDLimit),
		"--memory", strconv.Itoa(spec.Environment.MemoryMB) + "m",
		"--cpus", strconv.FormatFloat(float64(spec.Environment.CPUMillis)/1000, 'f', 3, 64),
		"--tmpfs", "/tmp:rw,nosuid,nodev,size=256m",
		"--tmpfs", "/home/ayati:rw,nosuid,nodev,size=512m,uid=1000,gid=1000",
		"--tmpfs", "/run/ayati:rw,nosuid,nodev,size=64m,uid=1000,gid=1000",
		"--mount", workspaceMount,
		"--mount", "type=bind,src=" + spec.CachePath + ",dst=/cache",
		"--workdir", "/workspace", spec.Environment.ImageDigest,
	}
}
