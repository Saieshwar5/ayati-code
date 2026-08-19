package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Saieshwar5/perpetual/internal/environment"
)

type dockerRuntimeMetadata struct {
	ID    string `json:"Id"`
	Image string `json:"Image"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
	Config struct {
		Image      string            `json:"Image"`
		User       string            `json:"User"`
		WorkingDir string            `json:"WorkingDir"`
		Labels     map[string]string `json:"Labels"`
	} `json:"Config"`
	HostConfig struct {
		ReadonlyRootfs bool              `json:"ReadonlyRootfs"`
		PidsLimit      int64             `json:"PidsLimit"`
		Memory         int64             `json:"Memory"`
		NanoCPUs       int64             `json:"NanoCpus"`
		NetworkMode    string            `json:"NetworkMode"`
		CapDrop        []string          `json:"CapDrop"`
		SecurityOpt    []string          `json:"SecurityOpt"`
		Tmpfs          map[string]string `json:"Tmpfs"`
		RestartPolicy  struct {
			Name string `json:"Name"`
		} `json:"RestartPolicy"`
	} `json:"HostConfig"`
	Mounts []struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		RW          bool   `json:"RW"`
	} `json:"Mounts"`
}

var errWorkspaceAccessMismatch = errors.New("workspace mount access does not match the lease")

func (d *DockerDriver) inspect(
	ctx context.Context, spec environment.RuntimeSpec, target string,
) (environment.Runtime, bool, error) {
	result, err := d.runner.Run(ctx, "container", "inspect", "--format", "{{json .}}", target)
	if err != nil {
		if strings.Contains(result.stderr, "No such container") || strings.Contains(result.stderr, "No such object") {
			return environment.Runtime{}, false, nil
		}
		message := strings.TrimSpace(result.stderr)
		if message == "" {
			message = err.Error()
		}
		return environment.Runtime{}, false, fmt.Errorf("inspect environment runtime: %s", message)
	}
	if result.truncated {
		return environment.Runtime{}, false, errors.New("inspect environment runtime: Docker metadata was truncated")
	}
	var metadata dockerRuntimeMetadata
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.stdout)), &metadata); err != nil {
		return environment.Runtime{}, false, fmt.Errorf("inspect environment runtime metadata: %w", err)
	}
	runtime, err := verifyRuntimeMetadata(spec, metadata)
	if err != nil {
		return environment.Runtime{}, false, err
	}
	return runtime, true, nil
}

func verifyRuntimeMetadata(
	spec environment.RuntimeSpec, metadata dockerRuntimeMetadata,
) (environment.Runtime, error) {
	if !validDockerID(metadata.ID) {
		return environment.Runtime{}, runtimeMismatch("invalid Docker identity")
	}
	for name, expected := range runtimeLabels(spec) {
		if metadata.Config.Labels[name] != expected {
			return environment.Runtime{}, runtimeMismatch("label %s is %q, expected %q",
				name, metadata.Config.Labels[name], expected)
		}
	}
	if metadata.Config.Image != spec.Environment.ImageDigest || metadata.Image != spec.Environment.ImageDigest {
		return environment.Runtime{}, runtimeMismatch("image identity does not match the environment")
	}
	if !metadata.HostConfig.ReadonlyRootfs || metadata.Config.User != "1000:1000" ||
		metadata.Config.WorkingDir != "/workspace" {
		return environment.Runtime{}, runtimeMismatch("root, user, or working directory policy does not match")
	}
	if metadata.HostConfig.PidsLimit != int64(spec.Environment.PIDLimit) ||
		metadata.HostConfig.Memory != int64(spec.Environment.MemoryMB)*1024*1024 ||
		metadata.HostConfig.NanoCPUs != int64(spec.Environment.CPUMillis)*1_000_000 {
		return environment.Runtime{}, runtimeMismatch("resource limits do not match the environment")
	}
	wantedNetwork := "bridge"
	if spec.Environment.NetworkPolicy == environment.NetworkDisabled {
		wantedNetwork = "none"
	}
	if metadata.HostConfig.NetworkMode != wantedNetwork {
		return environment.Runtime{}, runtimeMismatch("network mode is %q, expected %q",
			metadata.HostConfig.NetworkMode, wantedNetwork)
	}
	if !containsFold(metadata.HostConfig.CapDrop, "ALL") ||
		!containsPrefix(metadata.HostConfig.SecurityOpt, "no-new-privileges") ||
		metadata.HostConfig.RestartPolicy.Name != "no" {
		return environment.Runtime{}, runtimeMismatch("capability, privilege, or restart policy does not match")
	}
	if err := verifyRuntimeTmpfs(metadata.HostConfig.Tmpfs); err != nil {
		return environment.Runtime{}, err
	}
	if err := verifyRuntimeMounts(spec, metadata.Mounts); err != nil {
		return environment.Runtime{}, err
	}
	return environment.Runtime{
		ID: metadata.ID, Name: runtimeName(spec), EnvironmentID: spec.Environment.ID,
		WorkspaceID: spec.Lease.WorkspaceID, LeaseID: spec.Lease.ID,
		Generation: spec.Lease.Generation, ImageID: metadata.Image,
		Running: metadata.State.Running, WorkspaceWritable: spec.WorkspaceWritable,
	}, nil
}

func verifyRuntimeTmpfs(values map[string]string) error {
	required := map[string][]string{
		"/tmp":            {"rw", "nosuid", "nodev"},
		"/home/perpetual": {"rw", "nosuid", "nodev", "uid=1000", "gid=1000"},
		"/run/perpetual":  {"rw", "nosuid", "nodev", "uid=1000", "gid=1000"},
	}
	for path, options := range required {
		actual := strings.Split(values[path], ",")
		for _, option := range options {
			if !containsFold(actual, option) {
				return runtimeMismatch("tmpfs %s is missing %s", path, option)
			}
		}
	}
	return nil
}

func verifyRuntimeMounts(spec environment.RuntimeSpec, mounts []struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	RW          bool   `json:"RW"`
}) error {
	found := map[string]bool{}
	for _, mount := range mounts {
		if mount.Type == "tmpfs" {
			if mount.Destination != "/tmp" && mount.Destination != "/home/perpetual" &&
				mount.Destination != "/run/perpetual" {
				return runtimeMismatch("unexpected tmpfs mount %s", mount.Destination)
			}
			continue
		}
		if mount.Type != "bind" {
			return runtimeMismatch("mount %s is not a managed bind", mount.Destination)
		}
		switch mount.Destination {
		case "/workspace":
			if filepath.Clean(mount.Source) != spec.WorkspacePath {
				return runtimeMismatch("workspace mount does not match the lease")
			}
			if mount.RW != spec.WorkspaceWritable {
				return errWorkspaceAccessMismatch
			}
		case "/cache":
			if filepath.Clean(mount.Source) != spec.CachePath || !mount.RW {
				return runtimeMismatch("cache mount does not match the workspace")
			}
		default:
			return runtimeMismatch("unexpected persistent mount %s", mount.Destination)
		}
		found[mount.Destination] = true
	}
	if !found["/workspace"] || !found["/cache"] {
		return runtimeMismatch("workspace or cache mount is missing")
	}
	return nil
}

func containsFold(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(value, wanted) {
			return true
		}
	}
	return false
}

func containsPrefix(values []string, wanted string) bool {
	for _, value := range values {
		if strings.HasPrefix(strings.ToLower(value), strings.ToLower(wanted)) {
			return true
		}
	}
	return false
}

func runtimeMismatch(format string, arguments ...any) error {
	return fmt.Errorf("environment runtime configuration mismatch: "+format, arguments...)
}
