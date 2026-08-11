package shell

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

const (
	maxCommandBytes = 64 << 10
	maxOutputBytes  = 32 << 10
	defaultTimeout  = 2 * time.Minute
)

type Executor struct {
	workspace string
	timeout   time.Duration
}

func New(workspace string) (*Executor, error) {
	info, err := os.Stat(workspace)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("workspace is not a directory")
	}
	return &Executor{workspace: workspace, timeout: defaultTimeout}, nil
}

func (e *Executor) Run(ctx context.Context, command string) agent.ShellResult {
	started := time.Now()
	result := agent.ShellResult{Command: command, ExitCode: -1}
	if len(command) == 0 {
		result.Error = "shell command is empty"
		return result
	}
	if len(command) > maxCommandBytes {
		result.Error = "shell command exceeds 64 KiB"
		return result
	}
	callContext, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	stdout := &boundedBuffer{limit: maxOutputBytes}
	stderr := &boundedBuffer{limit: maxOutputBytes}
	process := exec.CommandContext(callContext, "/bin/sh", "-lc", command)
	process.Dir = e.workspace
	process.Env = childEnvironment()
	process.Stdout = stdout
	process.Stderr = stderr
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	process.Cancel = func() error {
		if process.Process == nil {
			return nil
		}
		return syscall.Kill(-process.Process.Pid, syscall.SIGKILL)
	}
	process.WaitDelay = time.Second
	err := process.Run()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.Truncated = stdout.truncated || stderr.truncated
	result.Duration = time.Since(started)
	result.TimedOut = errors.Is(callContext.Err(), context.DeadlineExceeded)
	canceled := errors.Is(callContext.Err(), context.Canceled)
	if err == nil {
		result.ExitCode = 0
		return result
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
		if result.TimedOut {
			result.Error = "shell command timed out"
		} else if canceled {
			result.Error = "shell command canceled"
		}
		return result
	}
	if canceled {
		result.Error = "shell command canceled"
		return result
	}
	result.Error = err.Error()
	return result
}

type boundedBuffer struct {
	data      []byte
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	available := b.limit - len(b.data)
	if available > 0 {
		if available > len(value) {
			available = len(value)
		}
		b.data = append(b.data, value[:available]...)
	}
	if available < len(value) {
		b.truncated = true
	}
	return len(value), nil
}

func (b *boundedBuffer) String() string {
	return string(b.data)
}

func childEnvironment() []string {
	environment := make([]string, 0, len(os.Environ()))
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		if name == "FIREWORKS_API_KEY" {
			continue
		}
		environment = append(environment, item)
	}
	return environment
}
