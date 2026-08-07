package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Executor struct {
	WorkingDir string
	Timeout    time.Duration
	MaxOutput  int
	Env        []string
}

type Result struct {
	Command  string `json:"command"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code"`
	Duration string `json:"duration"`
	TimedOut bool   `json:"timed_out,omitempty"`
}

func (e *Executor) Run(ctx context.Context, command string) (Result, error) {
	if command == "" {
		return Result{}, fmt.Errorf("shell command is empty")
	}
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(commandCtx, "/bin/sh", "-lc", command)
	cmd.Dir = e.WorkingDir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = e.environment()
	cmd.WaitDelay = 100 * time.Millisecond
	started := time.Now()
	err := cmd.Run()

	result := Result{
		Command:  command,
		Stdout:   truncate(stdout.String(), e.outputLimit()),
		Stderr:   truncate(stderr.String(), e.outputLimit()),
		ExitCode: 0,
		Duration: time.Since(started).Round(time.Millisecond).String(),
		TimedOut: errors.Is(commandCtx.Err(), context.DeadlineExceeded),
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else if result.TimedOut {
			result.ExitCode = 124
		} else {
			return result, fmt.Errorf("start shell: %w", err)
		}
	}
	return result, nil
}

func (r Result) JSON() string {
	encoded, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf(`{"command":%q,"exit_code":%d,"error":%q}`, r.Command, r.ExitCode, err.Error())
	}
	return string(encoded)
}

func (e *Executor) outputLimit() int {
	if e.MaxOutput <= 0 {
		return 32 << 10
	}
	return e.MaxOutput
}

func (e *Executor) environment() []string {
	environment := e.Env
	if environment == nil {
		environment = os.Environ()
	}
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if name == "FIREWORKS_API_KEY" {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	half := limit / 2
	return value[:half] + "\n... output truncated ...\n" + value[len(value)-half:]
}
