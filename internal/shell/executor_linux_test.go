//go:build linux

package shell

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExecutorBoundsOutputWhileCapturing(t *testing.T) {
	executor := Executor{WorkingDir: t.TempDir(), MaxOutput: 20, Env: []string{"PATH=/usr/bin:/bin"}}
	result, err := executor.Run(context.Background(), "printf 'abcdefghijklmnopqrstuvwxyz'")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.StdoutTruncated || result.StdoutBytes != 26 || !strings.HasPrefix(result.Stdout, "abcdefghij") || !strings.HasSuffix(result.Stdout, "qrstuvwxyz") {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExecutorTreatsNonzeroExitAsObservation(t *testing.T) {
	executor := Executor{WorkingDir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"}}
	result, err := executor.Run(context.Background(), "printf failure >&2; exit 7")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode != 7 || result.Stderr != "failure" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExecutorTimeoutKillsShellProcessGroup(t *testing.T) {
	executor := Executor{WorkingDir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	result, err := executor.Run(ctx, "sleep 5 & wait")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.TimedOut || result.ExitCode != 124 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("process group did not stop promptly: %s", time.Since(started))
	}
}

func TestExecutorDefaultEnvironmentDoesNotExposeArbitrarySecrets(t *testing.T) {
	t.Setenv("AYATI_TEST_SECRET", "should-not-leak")
	executor := Executor{WorkingDir: t.TempDir()}
	result, err := executor.Run(context.Background(), "env")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(result.Stdout, "AYATI_TEST_SECRET") || strings.Contains(result.Stdout, "should-not-leak") {
		t.Fatalf("secret leaked into shell environment: %s", result.Stdout)
	}
}
