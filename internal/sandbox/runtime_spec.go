package sandbox

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Saieshwar5/perpetual/internal/environment"
)

func normalizeRuntimeSpec(spec environment.RuntimeSpec) (environment.RuntimeSpec, error) {
	value, lease := spec.Environment, spec.Lease
	if value.Driver != environment.DriverDocker || value.ID == "" || lease.ID == "" ||
		lease.EnvironmentID != value.ID || lease.WorkspaceID == "" || lease.Generation < 1 ||
		lease.Generation != value.Generation {
		return environment.RuntimeSpec{}, errors.New("environment runtime identity is invalid")
	}
	for _, id := range []string{value.ID, lease.ID, lease.WorkspaceID} {
		if !validLabelID(id) {
			return environment.RuntimeSpec{}, errors.New("environment runtime identity is unsafe")
		}
	}
	if len(runtimeName(spec)) > 80 {
		return environment.RuntimeSpec{}, errors.New("environment runtime name is too long")
	}
	if !validImageID(value.ImageDigest) {
		return environment.RuntimeSpec{}, errors.New("environment image digest must be a resolved sha256 identity")
	}
	if value.CPUMillis < 100 || value.MemoryMB < 128 || value.PIDLimit < 16 ||
		(value.NetworkPolicy != environment.NetworkDisabled && value.NetworkPolicy != environment.NetworkOutbound) {
		return environment.RuntimeSpec{}, errors.New("environment runtime policy is invalid")
	}
	path, err := filepath.Abs(strings.TrimSpace(spec.WorkspacePath))
	if err != nil || strings.TrimSpace(spec.WorkspacePath) == "" {
		return environment.RuntimeSpec{}, errors.New("workspace path is required")
	}
	cache, err := filepath.Abs(strings.TrimSpace(spec.CachePath))
	if err != nil || strings.TrimSpace(spec.CachePath) == "" {
		return environment.RuntimeSpec{}, errors.New("workspace cache path is required")
	}
	relative, err := filepath.Rel(path, cache)
	if err != nil || relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return environment.RuntimeSpec{}, errors.New("workspace cache must be outside the project directory")
	}
	spec.WorkspacePath, spec.CachePath = filepath.Clean(path), filepath.Clean(cache)
	return spec, nil
}

func runtimeName(spec environment.RuntimeSpec) string {
	return runtimeNamePrefix + spec.Environment.ID + "-g" + strconv.FormatInt(spec.Lease.Generation, 10)
}

func legacyRuntimeName(spec environment.RuntimeSpec) string {
	return legacyRuntimeNamePrefix + spec.Environment.ID + "-g" + strconv.FormatInt(spec.Lease.Generation, 10)
}

func runtimeTarget(spec environment.RuntimeSpec, runtimeID string) (string, error) {
	runtimeID = strings.TrimSpace(runtimeID)
	if runtimeID == "" {
		return runtimeName(spec), nil
	}
	if !validDockerID(runtimeID) {
		return "", errors.New("environment runtime identity is unsafe")
	}
	return runtimeID, nil
}

func validLabelID(value string) bool {
	if value == "" || len(value) > 80 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func validRuntimeName(value string) bool {
	return (strings.HasPrefix(value, runtimeNamePrefix) || strings.HasPrefix(value, legacyRuntimeNamePrefix)) &&
		len(value) <= 80 && validLabelID(value)
}

func validImageID(value string) bool {
	if !strings.HasPrefix(value, "sha256:") {
		return false
	}
	value = strings.TrimPrefix(value, "sha256:")
	return len(value) == 64 && validHex(value)
}

func validDockerID(value string) bool {
	return len(value) >= 12 && len(value) <= 64 && validHex(value)
}

func validHex(value string) bool {
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return value != ""
}

func runtimeLabels(spec environment.RuntimeSpec) map[string]string {
	return map[string]string{
		labelManaged: "true", labelEnvironment: spec.Environment.ID,
		labelWorkspace: spec.Lease.WorkspaceID, labelLease: spec.Lease.ID,
		labelGeneration: strconv.FormatInt(spec.Lease.Generation, 10),
	}
}

func legacyRuntimeLabels(spec environment.RuntimeSpec) map[string]string {
	return map[string]string{
		legacyLabelManaged: "true", legacyLabelEnvironment: spec.Environment.ID,
		legacyLabelWorkspace: spec.Lease.WorkspaceID, legacyLabelLease: spec.Lease.ID,
		legacyLabelGeneration: strconv.FormatInt(spec.Lease.Generation, 10),
	}
}
