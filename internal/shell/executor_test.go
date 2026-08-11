package shell

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

func TestExecutorRunsInWorkspaceAndStripsProviderKey(t *testing.T) {
	t.Setenv("FIREWORKS_API_KEY", "must-not-enter-child")
	executor, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	result := executor.Run(context.Background(), `test -z "${FIREWORKS_API_KEY:-}" && pwd`)
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, executor.workspace) {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecutorBoundsOutput(t *testing.T) {
	executor, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	result := executor.Run(context.Background(), `yes x | head -c 40000`)
	if result.ExitCode != 0 || !result.Truncated || len(result.Stdout) != maxOutputBytes {
		t.Fatalf("result = %#v, stdout bytes = %d", result, len(result.Stdout))
	}
}

func TestExecutorTimesOutProcessGroup(t *testing.T) {
	executor, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	executor.timeout = 30 * time.Millisecond
	result := executor.Run(context.Background(), `sleep 10`)
	if !result.TimedOut || result.ExitCode == 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecutorStopsWhenContextIsCanceled(t *testing.T) {
	workspace := t.TempDir()
	executor, err := New(workspace)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	resultChannel := make(chan agent.ShellResult, 1)
	go func() {
		resultChannel <- executor.Run(ctx, `touch started; sleep 10`)
	}()
	marker := filepath.Join(workspace, "started")
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("shell command did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	result := <-resultChannel
	if result.ExitCode == 0 || result.Error != "shell command canceled" {
		t.Fatalf("result = %#v", result)
	}
}
