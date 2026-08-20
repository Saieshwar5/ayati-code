package exec

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/Saieshwar5/perpetual/internal/agent"
)

const (
	commandLimit   = 64 << 10
	outputLimit    = 32 << 10
	commandTimeout = 2 * time.Minute
)

// Shell executes bounded commands directly on the local machine.
type Shell struct {
	variables map[string]string
	env       []string
	dir       string
	timeout   time.Duration
}

// New validates and copies the given environment and returns a bounded local
// shell that runs commands with dir as its working directory. When no
// variables are supplied only PATH is inherited from the controller; the full
// host environment is never passed so credentials stay out of shell commands.
func New(variables map[string]string, dir string) (*Shell, error) {
	if err := validateVariables(variables); err != nil {
		return nil, err
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("shell working directory is required")
	}
	if len(variables) == 0 {
		if path := os.Getenv("PATH"); path != "" {
			variables = map[string]string{"PATH": path}
		}
	}
	return &Shell{
		variables: copyVariables(variables),
		env:       environmentSlice(variables),
		dir:       dir,
		timeout:   commandTimeout,
	}, nil
}

// Execute runs a single shell command with an output and timeout bound.
func (s *Shell) Execute(ctx context.Context, request agent.ShellRequest) agent.ShellResult {
	started := time.Now()
	result := agent.ShellResult{Command: request.Command, ExitCode: -1}
	command := strings.TrimSpace(request.Command)
	if command == "" {
		result.Error = "shell command is empty"
		return result
	}
	if len(command) > commandLimit {
		result.Error = "shell command exceeds 64 KiB"
		return result
	}
	callContext, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	stdout := &boundedBuffer{limit: outputLimit}
	stderr := &boundedBuffer{limit: outputLimit}
	cmd := exec.CommandContext(callContext, "/bin/sh", "-c", command)
	cmd.Dir = s.dir
	cmd.Env = s.env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	result.Stdout = redactEnvironment(stdout.String(), s.variables, stdout.truncated)
	result.Stderr = redactEnvironment(stderr.String(), s.variables, stderr.truncated)
	result.ExitCode = 0
	result.Truncated = stdout.truncated || stderr.truncated
	result.Duration = time.Since(started)
	if err != nil {
		result.ExitCode = -1
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			result.ExitCode = exitError.ExitCode()
		}
	}
	contextErr := callContext.Err()
	result.TimedOut = errors.Is(contextErr, context.DeadlineExceeded) || result.ExitCode == 124
	if result.TimedOut {
		result.Error = "shell command timed out"
	} else if errors.Is(contextErr, context.Canceled) {
		result.Error = "shell command canceled"
	} else if err != nil && result.ExitCode == -1 {
		result.Error = err.Error()
	}
	return result
}

func environmentSlice(variables map[string]string) []string {
	names := make([]string, 0, len(variables))
	for name := range variables {
		names = append(names, name)
	}
	sort.Strings(names)
	values := make([]string, 0, len(names))
	for _, name := range names {
		values = append(values, name+"="+variables[name])
	}
	return values
}
