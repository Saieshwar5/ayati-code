package workspaceruntime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/exec"
)

func TestLocalRuntimeLifecycleIsIdempotent(t *testing.T) {
	local := NewLocal()
	ref := Ref{ID: "workspace-1", Directory: t.TempDir(), CacheDir: t.TempDir()}
	if err := local.Start(context.Background(), ref); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := local.Stop(context.Background(), ref); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := local.Destroy(context.Background(), ref); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
}

func TestLocalRuntimeOpenShellUsesWorkspaceDirectoryAndPrivateHome(t *testing.T) {
	directory := t.TempDir()
	home := filepath.Join(t.TempDir(), "home")
	shell, err := NewLocal().OpenShell(context.Background(), Ref{
		ID: "workspace-1", Directory: directory, CacheDir: t.TempDir(),
	}, map[string]string{"HOME": home, "PATH": os.Getenv("PATH")})
	if err != nil {
		t.Fatalf("OpenShell: %v", err)
	}
	result := shell.Execute(context.Background(), exec.ShellRequest{
		Command: "pwd && printf '%s' \"$HOME\"",
	})
	if result.ExitCode != 0 || result.Error != "" {
		t.Fatalf("shell result = %#v", result)
	}
	if !strings.Contains(result.Stdout, directory) {
		t.Fatalf("stdout does not contain workspace directory: %#v", result.Stdout)
	}
	// "$HOME" is redacted from captured output by the bounded shell; the
	// private home directory is verified through the filesystem below.
	if strings.Contains(result.Stdout, home) {
		t.Fatalf("stdout leaked the private home value: %#v", result.Stdout)
	}
	if info, err := os.Stat(home); err != nil || !info.IsDir() {
		t.Fatalf("private home = %v, error = %v", info, err)
	}
}

func TestLocalRuntimeOpenShellRejectsEmptyDirectory(t *testing.T) {
	_, err := NewLocal().OpenShell(context.Background(), Ref{ID: "workspace-1"},
		map[string]string{"HOME": t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "directory is required") {
		t.Fatalf("OpenShell error = %v", err)
	}
}
