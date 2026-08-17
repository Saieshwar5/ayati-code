package agent

import (
	"context"
	"time"
)

const MaxSteps = 20

type ShellRequest struct {
	Command string `json:"command"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type Request struct {
	Model        string
	SystemPrompt string
	Messages     []Message
	DisableShell bool
}

type Provider interface {
	Next(context.Context, Request) (Message, error)
}

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

type Shell interface {
	Execute(context.Context, ShellRequest) ShellResult
}

type Recorder interface {
	Append(Message) error
}

type Observer interface {
	Step(current, maximum int)
	ToolCall(request ShellRequest)
	ToolResult(ShellResult)
	Assistant(text string)
}

type Completion struct {
	Text      string
	Steps     int
	Exhausted bool
}
