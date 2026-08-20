package exec

import (
	"context"
	"time"
)

// ShellRequest is a single bounded shell command.
type ShellRequest struct {
	Command string `json:"command"`
}

// ShellResult is the bounded outcome of one ShellRequest.
type ShellResult struct {
	Command   string        `json:"command,omitempty"`
	Stdout    string        `json:"stdout,omitempty"`
	Stderr    string        `json:"stderr,omitempty"`
	ExitCode  int           `json:"exit_code"`
	Duration  time.Duration `json:"duration"`
	TimedOut  bool          `json:"timed_out,omitempty"`
	Truncated bool          `json:"truncated,omitempty"`
	Error     string        `json:"error,omitempty"`
}

// Shell executes bounded commands; the built-in agent and workspace flows are
// the only consumers, and a future agent will reuse this contract.
type Shell interface {
	Execute(context.Context, ShellRequest) ShellResult
}
