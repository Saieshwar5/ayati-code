package environment_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/environment"
	"github.com/Saieshwar5/perpetual/internal/sandbox"
)

func TestEnvironmentManagementDockerIntegration(t *testing.T) {
	if os.Getenv("PERPETUAL_DOCKER_INTEGRATION") != "1" {
		t.Skip("set PERPETUAL_DOCKER_INTEGRATION=1 to exercise Docker")
	}
	_, _, store := openStores(t)
	driver, err := sandbox.NewDockerDriver()
	if err != nil {
		t.Fatalf("NewDockerDriver: %v", err)
	}
	service, err := environment.NewManagementService(store, driver)
	if err != nil {
		t.Fatalf("NewManagementService: %v", err)
	}
	value, err := service.Create(context.Background(), environment.CreateInput{
		Name: "Managed Docker integration", ImageRef: sandbox.DefaultImage,
		CPUMillis: 1000, MemoryMB: 1024, PIDLimit: 128,
	})
	if err != nil || value.State != environment.StateAvailable ||
		!strings.HasPrefix(value.ImageDigest, "sha256:") {
		t.Fatalf("environment = %#v, error = %v", value, err)
	}
	if err := service.Delete(context.Background(), value.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}
