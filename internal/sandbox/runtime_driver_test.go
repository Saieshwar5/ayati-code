package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/ayati-code/internal/environment"
)

func TestDockerDriverCreatesVerifiedLeaseRuntime(t *testing.T) {
	spec := testRuntimeSpec(t, false, environment.NetworkDisabled)
	runtimeID := strings.Repeat("f", 64)
	runner := &fakeRunner{
		results: []commandResult{
			{stderr: "No such container"}, {stdout: runtimeID + "\n"}, {},
			{stdout: runtimeMetadata(t, spec, runtimeID, true, nil)},
		},
		errors: []error{errors.New("missing")},
	}
	driver := &DockerDriver{runner: runner}
	runtime, err := driver.Create(context.Background(), spec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if runtime.ID != runtimeID || runtime.Running != true || runtime.WorkspaceWritable {
		t.Fatalf("runtime = %#v", runtime)
	}
	if len(runner.calls) != 4 {
		t.Fatalf("calls = %#v", runner.calls)
	}
	create := strings.Join(runner.calls[1], " ")
	for _, expected := range []string{
		"--label ayati.runtime=true", "--label ayati.environment=" + spec.Environment.ID,
		"--label ayati.workspace=" + spec.Lease.WorkspaceID,
		"--label ayati.lease=" + spec.Lease.ID, "--label ayati.generation=3",
		"--read-only", "--cap-drop ALL", "--security-opt no-new-privileges",
		"--network none", "--pids-limit 384", "--memory 6144m", "--cpus 3.500",
		"/run/ayati:rw,nosuid,nodev", "dst=/workspace,readonly", "dst=/cache",
		spec.Environment.ImageDigest,
	} {
		if !strings.Contains(create, expected) {
			t.Fatalf("create arguments %q do not contain %q", create, expected)
		}
	}
}

func TestDockerDriverRestartsMatchingRuntime(t *testing.T) {
	spec := testRuntimeSpec(t, true, environment.NetworkOutbound)
	runtimeID := strings.Repeat("e", 64)
	runner := &fakeRunner{results: []commandResult{
		{stdout: runtimeMetadata(t, spec, runtimeID, false, nil)}, {},
		{stdout: runtimeMetadata(t, spec, runtimeID, true, nil)},
	}}
	driver := &DockerDriver{runner: runner}
	runtime, err := driver.Create(context.Background(), spec)
	if err != nil || !runtime.Running || !runtime.WorkspaceWritable {
		t.Fatalf("runtime = %#v, error = %v", runtime, err)
	}
	if len(runner.calls) != 3 || runner.calls[1][0] != "start" || runner.calls[1][1] != runtimeID {
		t.Fatalf("calls = %#v", runner.calls)
	}
}

func TestDockerDriverCleansNewRuntimeWhenVerificationFails(t *testing.T) {
	spec := testRuntimeSpec(t, false, environment.NetworkOutbound)
	runtimeID := strings.Repeat("d", 64)
	runner := &fakeRunner{
		results: []commandResult{
			{stderr: "No such object"}, {stdout: runtimeID}, {},
			{stdout: runtimeMetadata(t, spec, runtimeID, true, func(value map[string]any) {
				value["HostConfig"].(map[string]any)["Memory"] = int64(1)
			})}, {},
		},
		errors: []error{errors.New("missing")},
	}
	driver := &DockerDriver{runner: runner}
	_, err := driver.Create(context.Background(), spec)
	if err == nil || !strings.Contains(err.Error(), "resource limits") {
		t.Fatalf("Create error = %v", err)
	}
	last := runner.calls[len(runner.calls)-1]
	if len(last) != 4 || last[0] != "rm" || last[3] != runtimeID {
		t.Fatalf("cleanup call = %#v", last)
	}
}

func TestDockerDriverCleansNamedRuntimeWhenDockerReturnsInvalidIdentity(t *testing.T) {
	spec := testRuntimeSpec(t, false, environment.NetworkOutbound)
	runner := &fakeRunner{
		results: []commandResult{{stderr: "No such container"}, {stdout: "not-an-id"}, {}},
		errors:  []error{errors.New("missing")},
	}
	driver := &DockerDriver{runner: runner}
	_, err := driver.Create(context.Background(), spec)
	if err == nil || !strings.Contains(err.Error(), "invalid identity") {
		t.Fatalf("Create error = %v", err)
	}
	last := runner.calls[len(runner.calls)-1]
	if len(last) != 4 || last[0] != "rm" || last[3] != runtimeName(spec) {
		t.Fatalf("cleanup call = %#v", last)
	}
}

func TestDockerDriverRefusesRuntimeOwnedByAnotherLease(t *testing.T) {
	spec := testRuntimeSpec(t, false, environment.NetworkOutbound)
	runtimeID := strings.Repeat("c", 64)
	runner := &fakeRunner{results: []commandResult{{
		stdout: runtimeMetadata(t, spec, runtimeID, true, func(value map[string]any) {
			value["Config"].(map[string]any)["Labels"].(map[string]string)[labelLease] = strings.Repeat("9", 24)
		}),
	}}}
	driver := &DockerDriver{runner: runner}
	if err := driver.Destroy(context.Background(), spec, runtimeID); err == nil ||
		!strings.Contains(err.Error(), "label "+labelLease) {
		t.Fatalf("Destroy error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %#v", runner.calls)
	}
}

func TestDockerDriverRejectsUnexpectedHostMount(t *testing.T) {
	spec := testRuntimeSpec(t, false, environment.NetworkOutbound)
	runtimeID := strings.Repeat("8", 64)
	runner := &fakeRunner{results: []commandResult{{
		stdout: runtimeMetadata(t, spec, runtimeID, true, func(value map[string]any) {
			mounts := value["Mounts"].([]map[string]any)
			value["Mounts"] = append(mounts, map[string]any{
				"Type": "bind", "Source": "/var/run/docker.sock",
				"Destination": "/var/run/docker.sock", "RW": true,
			})
		}),
	}}}
	driver := &DockerDriver{runner: runner}
	_, _, err := driver.Inspect(context.Background(), spec, runtimeID)
	if err == nil || !strings.Contains(err.Error(), "unexpected persistent mount") {
		t.Fatalf("Inspect error = %v", err)
	}
}

func TestDockerDriverDestroysOnlyVerifiedRuntime(t *testing.T) {
	spec := testRuntimeSpec(t, true, environment.NetworkOutbound)
	runtimeID := strings.Repeat("b", 64)
	runner := &fakeRunner{
		results: []commandResult{
			{stdout: runtimeMetadata(t, spec, runtimeID, true, nil)}, {},
			{stderr: "No such container"},
		},
		errors: []error{nil, nil, errors.New("missing")},
	}
	driver := &DockerDriver{runner: runner}
	if err := driver.Destroy(context.Background(), spec, runtimeID); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if len(runner.calls) != 3 || runner.calls[1][0] != "rm" || runner.calls[1][3] != runtimeID {
		t.Fatalf("calls = %#v", runner.calls)
	}
}

func testRuntimeSpec(t *testing.T, writable bool, network string) environment.RuntimeSpec {
	t.Helper()
	root := t.TempDir()
	workspacePath := filepath.Join(root, "repo")
	return environment.RuntimeSpec{
		Environment: environment.Environment{
			ID: strings.Repeat("1", 24), Driver: environment.DriverDocker,
			ImageRef: "ayati-sandbox:dev", ImageDigest: "sha256:" + strings.Repeat("a", 64),
			CPUMillis: 3500, MemoryMB: 6144, PIDLimit: 384, NetworkPolicy: network,
			ProvisioningState: environment.ProvisioningReady, Generation: 3,
		},
		Lease: environment.Lease{
			ID: strings.Repeat("2", 24), EnvironmentID: strings.Repeat("1", 24),
			WorkspaceID: strings.Repeat("3", 24), Generation: 3, State: environment.LeaseAcquiring,
		},
		WorkspacePath: workspacePath, CachePath: filepath.Join(root, "cache"),
		WorkspaceWritable: writable,
	}
}

func runtimeMetadata(
	t *testing.T, spec environment.RuntimeSpec, runtimeID string, running bool,
	mutate func(map[string]any),
) string {
	t.Helper()
	network := "bridge"
	if spec.Environment.NetworkPolicy == environment.NetworkDisabled {
		network = "none"
	}
	value := map[string]any{
		"Id": runtimeID, "Image": spec.Environment.ImageDigest,
		"State": map[string]any{"Running": running},
		"Config": map[string]any{
			"Image": spec.Environment.ImageDigest, "User": "1000:1000", "WorkingDir": "/workspace",
			"Labels": runtimeLabels(spec),
		},
		"HostConfig": map[string]any{
			"ReadonlyRootfs": true, "PidsLimit": int64(spec.Environment.PIDLimit),
			"Memory":      int64(spec.Environment.MemoryMB) * 1024 * 1024,
			"NanoCpus":    int64(spec.Environment.CPUMillis) * 1_000_000,
			"NetworkMode": network, "CapDrop": []string{"ALL"},
			"SecurityOpt": []string{"no-new-privileges:true"},
			"Tmpfs": map[string]string{
				"/tmp": "rw,nosuid,nodev,size=256m", "/home/ayati": "rw,nosuid,nodev,size=512m,uid=1000,gid=1000",
				"/run/ayati": "rw,nosuid,nodev,size=64m,uid=1000,gid=1000",
			},
			"RestartPolicy": map[string]any{"Name": "no"},
		},
		"Mounts": []map[string]any{
			{"Type": "bind", "Source": spec.WorkspacePath, "Destination": "/workspace", "RW": spec.WorkspaceWritable},
			{"Type": "bind", "Source": spec.CachePath, "Destination": "/cache", "RW": true},
		},
	}
	if mutate != nil {
		mutate(value)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal runtime metadata: %v", err)
	}
	return string(data)
}
