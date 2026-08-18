package sandbox

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Saieshwar5/perpetual/internal/agent"
)

const (
	DefaultImage   = "perpetual-sandbox:dev"
	commandLimit   = 64 << 10
	outputLimit    = 32 << 10
	commandTimeout = 2 * time.Minute
)

type Shell struct {
	runner    runner
	stop      func(context.Context, string) error
	name      string
	timeout   time.Duration
	variables map[string]string
}

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
	seconds := strconv.Itoa(int(s.timeout.Seconds()))
	input, arguments := environmentCommand(s.name, seconds, command, s.variables)
	output, err := s.runner.RunInput(callContext, input, arguments...)
	contextErr := callContext.Err()
	var stopErr error
	if contextErr != nil {
		stopContext, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		stopErr = s.stop(stopContext, s.name)
		stopCancel()
	}
	result.Stdout = redactEnvironment(output.stdout, s.variables, output.truncated)
	result.Stderr = redactEnvironment(output.stderr, s.variables, output.truncated)
	result.ExitCode = output.exitCode
	result.Truncated = output.truncated
	result.Duration = time.Since(started)
	result.TimedOut = errors.Is(contextErr, context.DeadlineExceeded) || output.exitCode == 124
	if result.TimedOut {
		result.Error = "shell command timed out"
	} else if errors.Is(contextErr, context.Canceled) {
		result.Error = "shell command canceled"
	} else if err != nil && output.exitCode == -1 {
		result.Error = err.Error()
	}
	if stopErr != nil {
		result.Error = strings.TrimSpace(result.Error + "; stop sandbox: " + stopErr.Error())
	}
	return result
}
