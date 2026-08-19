package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Saieshwar5/perpetual/internal/environment"
)

func TestDockerEnvironmentRuntimeIntegration(t *testing.T) {
	if os.Getenv("PERPETUAL_DOCKER_INTEGRATION") != "1" {
		t.Skip("set PERPETUAL_DOCKER_INTEGRATION=1 to exercise Docker")
	}
	driver, err := NewDockerDriver()
	if err != nil {
		t.Fatalf("NewDockerDriver: %v", err)
	}
	image, err := driver.ResolveImage(context.Background(), DefaultImage)
	if err != nil {
		t.Fatalf("resolve sandbox image: %v", err)
	}
	identity := fmt.Sprintf("%024x", time.Now().UnixNano())
	root := t.TempDir()
	spec := environment.RuntimeSpec{
		Environment: environment.Environment{
			ID: identity, Driver: environment.DriverDocker, ImageRef: DefaultImage,
			ImageDigest: image, CPUMillis: 1000, MemoryMB: 1024,
			PIDLimit: 128, NetworkPolicy: environment.NetworkDisabled,
			ProvisioningState: environment.ProvisioningReady, Generation: 1,
		},
		Lease: environment.Lease{
			ID: "1" + identity[1:], EnvironmentID: identity, WorkspaceID: "2" + identity[1:],
			Generation: 1, State: environment.LeaseAcquiring,
		},
		WorkspacePath: filepath.Join(root, "repo"), CachePath: filepath.Join(root, "cache"),
	}
	runtime, err := driver.Create(context.Background(), spec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = driver.Destroy(context.Background(), spec, runtime.ID) })
	result, err := driver.run(context.Background(), "exec", runtime.ID, "/bin/sh", "-c", strings.Join([]string{
		`test "$(id -u)" = 1000`, `! touch /workspace/blocked`, `touch /cache/allowed`,
		`touch /tmp/allowed`, `touch /run/perpetual/allowed`, `test ! -e /var/run/docker.sock`,
	}, " && "))
	if err != nil {
		t.Fatalf("verify runtime: %v: %s", err, result.stderr)
	}
	if err := driver.Destroy(context.Background(), spec, runtime.ID); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
}
