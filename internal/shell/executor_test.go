package shell

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunCapturesOutputAndExitCode(t *testing.T) {
	executor := Executor{WorkingDir: t.TempDir(), Timeout: time.Second}
	result, err := executor.Run(context.Background(), "printf hello; printf problem >&2; exit 7")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Stdout != "hello" || result.Stderr != "problem" || result.ExitCode != 7 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRunTimesOut(t *testing.T) {
	executor := Executor{WorkingDir: t.TempDir(), Timeout: 20 * time.Millisecond}
	result, err := executor.Run(context.Background(), "sleep 2")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.TimedOut || result.ExitCode == 0 {
		t.Fatalf("expected timeout result, got %+v", result)
	}
}

func TestRunTruncatesLargeOutput(t *testing.T) {
	executor := Executor{WorkingDir: t.TempDir(), Timeout: time.Second, MaxOutput: 100}
	result, err := executor.Run(context.Background(), "yes x | head -n 100")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(result.Stdout, "output truncated") {
		t.Fatalf("expected truncation marker, got %q", result.Stdout)
	}
}

func TestRunStripsFireworksAPIKey(t *testing.T) {
	executor := Executor{
		WorkingDir: t.TempDir(),
		Timeout:    time.Second,
		Env:        []string{"PATH=/usr/bin:/bin", "FIREWORKS_API_KEY=top-secret", "SAFE_VALUE=visible"},
	}
	result, err := executor.Run(context.Background(), `printf '%s|%s' "$FIREWORKS_API_KEY" "$SAFE_VALUE"`)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Stdout != "|visible" {
		t.Fatalf("secret reached shell environment: %q", result.Stdout)
	}
}
