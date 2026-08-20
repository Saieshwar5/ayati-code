package exec

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestShellExecutesCommandInWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	shell, err := New(nil, dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	result := shell.Execute(context.Background(), ShellRequest{Command: "pwd"})
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != dir {
		t.Fatalf("result = %#v", result)
	}
}

func TestShellInjectsEnvironmentVariables(t *testing.T) {
	shell, err := New(map[string]string{"TEST_VALUE": "hello"}, t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	result := shell.Execute(context.Background(), ShellRequest{Command: "printf %s \"${#TEST_VALUE}\""})
	if result.ExitCode != 0 || result.Stdout != "5" {
		t.Fatalf("result = %#v", result)
	}
}

func TestShellRedactsConfiguredValues(t *testing.T) {
	shell, err := New(map[string]string{"SECRET": "12345678"}, t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	result := shell.Execute(context.Background(), ShellRequest{Command: "printf 12345678"})
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, "[REDACTED:SECRET]") {
		t.Fatalf("result = %#v", result)
	}
}

func TestShellReportsEmptyCommand(t *testing.T) {
	shell, err := New(nil, t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	result := shell.Execute(context.Background(), ShellRequest{Command: "  "})
	if result.Error != "shell command is empty" || result.ExitCode != -1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestShellRejectsOversizedCommand(t *testing.T) {
	shell, err := New(nil, t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	result := shell.Execute(context.Background(), ShellRequest{Command: strings.Repeat("x", commandLimit+1)})
	if result.Error != "shell command exceeds 64 KiB" {
		t.Fatalf("result = %#v", result)
	}
}

func TestShellTimesOutLongCommand(t *testing.T) {
	shell, err := New(nil, t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	shell.timeout = 100 * time.Millisecond
	started := time.Now()
	result := shell.Execute(context.Background(), ShellRequest{Command: "sleep 5"})
	if !result.TimedOut || result.Error != "shell command timed out" || time.Since(started) > 5*time.Second {
		t.Fatalf("result = %#v", result)
	}
}

func TestShellReportsExitCode(t *testing.T) {
	shell, err := New(nil, t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	result := shell.Execute(context.Background(), ShellRequest{Command: "exit 3"})
	if result.ExitCode != 3 {
		t.Fatalf("result = %#v", result)
	}
}

func TestShellValidatesEnvironmentNames(t *testing.T) {
	if _, err := New(map[string]string{"bad name": "value"}, t.TempDir()); err == nil {
		t.Fatal("New accepted an invalid variable name")
	}
	if _, err := New(map[string]string{"OK": "value\x00"}, t.TempDir()); err == nil {
		t.Fatal("New accepted a NUL byte")
	}
	if _, err := New(nil, ""); err == nil {
		t.Fatal("New accepted an empty working directory")
	}
}

func TestShellCancelsProcessGroup(t *testing.T) {
	shell, err := New(nil, t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var result ShellResult
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		result = shell.Execute(ctx, ShellRequest{Command: "sleep 30"})
	}()
	time.Sleep(200 * time.Millisecond)
	cancel()
	wg.Wait()
	if result.Error != "shell command canceled" {
		t.Fatalf("result = %#v", result)
	}
}

func TestShellParanoiaWorkingDirectoryOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	shell, err := New(nil, dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	result := shell.Execute(context.Background(), ShellRequest{Command: "pwd"})
	if filepath.Clean(strings.TrimSpace(result.Stdout)) != filepath.Clean(dir) {
		t.Fatalf("result = %#v", result)
	}
}
