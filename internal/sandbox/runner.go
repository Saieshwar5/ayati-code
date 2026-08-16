package sandbox

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type osRunner struct{ docker string }

func (r osRunner) Run(ctx context.Context, arguments ...string) (commandResult, error) {
	return r.RunInput(ctx, "", arguments...)
}

func (r osRunner) RunInput(ctx context.Context, input string, arguments ...string) (commandResult, error) {
	stdout := &boundedBuffer{limit: outputLimit}
	stderr := &boundedBuffer{limit: outputLimit}
	command := exec.CommandContext(ctx, r.docker, arguments...)
	if input != "" {
		command.Stdin = strings.NewReader(input)
	}
	command.Stdout = stdout
	command.Stderr = stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	command.WaitDelay = time.Second
	err := command.Run()
	result := commandResult{
		stdout: stdout.String(), stderr: stderr.String(), exitCode: 0,
		truncated: stdout.truncated || stderr.truncated,
	}
	if err == nil {
		return result, nil
	}
	result.exitCode = -1
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.exitCode = exitError.ExitCode()
	}
	return result, err
}

type boundedBuffer struct {
	data      []byte
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	available := b.limit - len(b.data)
	if available > len(value) {
		available = len(value)
	}
	if available > 0 {
		b.data = append(b.data, value[:available]...)
	}
	if available < len(value) {
		b.truncated = true
	}
	return len(value), nil
}

func (b *boundedBuffer) String() string { return string(b.data) }
