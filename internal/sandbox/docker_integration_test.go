package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

func TestDockerPersistentWorkspaceIntegration(t *testing.T) {
	if os.Getenv("AYATI_DOCKER_INTEGRATION") != "1" {
		t.Skip("set AYATI_DOCKER_INTEGRATION=1 to exercise Docker")
	}
	manager, err := New(DefaultImage)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	name := fmt.Sprintf("ayati-workspace-test-%d", time.Now().UnixNano())
	path := filepath.Join(t.TempDir(), "repo")
	ctx := context.Background()
	if _, err := manager.Ensure(ctx, Spec{Name: name, Path: path, MountMode: MountReadWrite}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	t.Cleanup(func() { _ = manager.Remove(context.Background(), name) })
	shell, err := manager.Open(name, map[string]string{"AYATI_TEST_VALUE": "sandbox-secret"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	command := strings.Join([]string{
		`test "$(id -u)" = 1000`,
		`test -z "$FIREWORKS_API_KEY"`,
		`command -v git`, `command -v go`, `command -v node`, `command -v npm`,
		`command -v python3`, `command -v rg`,
		`test "$AYATI_TEST_VALUE" = sandbox-secret`,
		`git init -q`, `git config user.name Ayati`, `git config user.email ayati@example.test`,
		`printf persistent > .ayati-integration`, `git add .ayati-integration`, `git commit -qm baseline`,
	}, " && ")
	result := shell.Execute(ctx, agent.ShellRequest{Command: command})
	if result.ExitCode != 0 || result.Error != "" {
		t.Fatalf("first command = %#v", result)
	}
	if _, err := manager.Ensure(ctx, Spec{Name: name, Path: path, MountMode: MountReadWrite}); err != nil {
		t.Fatalf("Ensure existing: %v", err)
	}
	result = shell.Execute(ctx, agent.ShellRequest{Command: "cat .ayati-integration && node --version && go version"})
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, "persistent") || !strings.Contains(result.Stdout, "v22.") {
		t.Fatalf("second command = %#v", result)
	}
	canceled, cancel := context.WithCancel(ctx)
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	result = shell.Execute(canceled, agent.ShellRequest{Command: "sleep 30"})
	if result.Error == "" || !strings.Contains(result.Error, "canceled") {
		t.Fatalf("canceled command = %#v", result)
	}
	if _, err := manager.Ensure(ctx, Spec{Name: name, Path: path, MountMode: MountReadWrite}); err != nil {
		t.Fatalf("restore canceled sandbox: %v", err)
	}
	result = shell.Execute(ctx, agent.ShellRequest{Command: "cat .ayati-integration"})
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "persistent" {
		t.Fatalf("restored command = %#v", result)
	}
	if err := manager.Remove(ctx, name); err != nil {
		t.Fatalf("Remove writable sandbox: %v", err)
	}
	if _, err := manager.Ensure(ctx, Spec{Name: name, Path: path, MountMode: MountReadOnly}); err != nil {
		t.Fatalf("Ensure Explore sandbox: %v", err)
	}
	result = shell.Execute(ctx, agent.ShellRequest{Command: strings.Join([]string{
		`rg persistent .ayati-integration`, `grep persistent .ayati-integration`, `cat .ayati-integration`,
		`git log -1 --oneline`, `touch /tmp/allowed`, `touch /cache/allowed`,
		`! touch blocked`, `! sh -c 'printf changed > .ayati-integration'`,
		`! sed -i s/persistent/changed/ .ayati-integration`, `! rm .ayati-integration`,
		`! git commit --allow-empty -m blocked`, `test -z "$(git status --porcelain)"`,
	}, " && ")})
	if result.ExitCode != 0 || result.Error != "" {
		t.Fatalf("Explore command = %#v", result)
	}
	if err := manager.Remove(ctx, name); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(path, ".ayati-integration")); err != nil || string(data) != "persistent" {
		t.Fatalf("workspace data = %q, error = %v", data, err)
	}
}
