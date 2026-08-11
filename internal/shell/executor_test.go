package shell

import (
	"context"
	"strings"
	"testing"
	"time"
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
