//go:build linux

package shell

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	agentruntime "github.com/Saieshwar5/ayati-runtime/internal/runtime"
)

type Executor struct {
	WorkingDir string
	ShellPath  string
	MaxOutput  int
	Env        []string
}

func (e *Executor) Run(ctx context.Context, command string) (agentruntime.ToolResult, error) {
	if strings.TrimSpace(command) == "" {
		return agentruntime.ToolResult{}, fmt.Errorf("shell command is empty")
	}
	shellPath := e.ShellPath
	if shellPath == "" {
		shellPath = "/bin/bash"
	}

	stdout := newBoundedBuffer(e.outputLimit())
	stderr := newBoundedBuffer(e.outputLimit())
	cmd := exec.CommandContext(ctx, shellPath, "-lc", command)
	cmd.Dir = e.WorkingDir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = e.environment()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 250 * time.Millisecond

	started := time.Now()
	err := cmd.Run()
	result := agentruntime.ToolResult{
		Stdout:          stdout.String(),
		Stderr:          stderr.String(),
		ExitCode:        0,
		DurationMS:      time.Since(started).Milliseconds(),
		TimedOut:        errors.Is(ctx.Err(), context.DeadlineExceeded),
		StdoutTruncated: stdout.Truncated(),
		StderrTruncated: stderr.Truncated(),
		StdoutBytes:     stdout.Total(),
		StderrBytes:     stderr.Total(),
	}
	if result.TimedOut {
		result.ExitCode = 124
		return result, nil
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return result, ctx.Err()
		}
		return result, fmt.Errorf("start shell: %w", err)
	}
	return result, nil
}

func (e *Executor) outputLimit() int {
	if e.MaxOutput <= 0 {
		return 16 << 10
	}
	return e.MaxOutput
}

func (e *Executor) environment() []string {
	if e.Env != nil {
		return append([]string(nil), e.Env...)
	}
	allowed := map[string]bool{
		"PATH": true, "HOME": true, "USER": true, "LOGNAME": true,
		"LANG": true, "LC_ALL": true, "LC_CTYPE": true, "TERM": true,
		"TMPDIR": true, "GOPATH": true, "GOCACHE": true, "GOMODCACHE": true,
		"CARGO_HOME": true, "RUSTUP_HOME": true,
	}
	result := make([]string, 0, len(allowed))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if allowed[name] {
			result = append(result, entry)
		}
	}
	return result
}
