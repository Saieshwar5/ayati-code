package runtime

import (
	"context"
	"encoding/json"
	"time"
)

const (
	ProtocolVersion = 1
	ShellToolName   = "shell"
)

type Request struct {
	Version   int    `json:"version,omitempty"`
	RunID     string `json:"run_id"`
	Prompt    string `json:"prompt"`
	Workspace string `json:"workspace"`
}

type Limits struct {
	MaxSteps            int
	MaxContextRollovers int
	RunTimeout          time.Duration
	ModelTimeout        time.Duration
	ShellTimeout        time.Duration
	MaxOutputBytes      int
}

type LimitSnapshot struct {
	MaxSteps                int   `json:"max_steps"`
	MaxContextRollovers     int   `json:"max_context_rollovers"`
	ContextWindowTokens     int   `json:"context_window_tokens,omitempty"`
	ContextCheckpointTokens int   `json:"context_checkpoint_tokens,omitempty"`
	RunTimeoutMS            int64 `json:"run_timeout_ms"`
	ModelTimeoutMS          int64 `json:"model_timeout_ms"`
	ShellTimeoutMS          int64 `json:"shell_timeout_ms"`
	MaxOutputBytes          int   `json:"max_tool_output_bytes"`
}

type ContextPolicy struct {
	WindowTokens    int
	MaxOutputTokens int
}

type Usage struct {
	InputTokens     int64 `json:"input_tokens,omitempty"`
	OutputTokens    int64 `json:"output_tokens,omitempty"`
	CachedTokens    int64 `json:"cached_tokens,omitempty"`
	ReasoningTokens int64 `json:"reasoning_tokens,omitempty"`
	TotalTokens     int64 `json:"total_tokens,omitempty"`
}

func (u *Usage) Add(other Usage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.CachedTokens += other.CachedTokens
	u.ReasoningTokens += other.ReasoningTokens
	u.TotalTokens += other.TotalTokens
}

type ToolDefinition struct {
	Name        string
	Description string
}

type ShellCall struct {
	Command string `json:"command"`
}

type ToolResult struct {
	Stdout          string `json:"stdout,omitempty"`
	Stderr          string `json:"stderr,omitempty"`
	ExitCode        int    `json:"exit_code"`
	DurationMS      int64  `json:"duration_ms"`
	TimedOut        bool   `json:"timed_out,omitempty"`
	StdoutTruncated bool   `json:"stdout_truncated,omitempty"`
	StderrTruncated bool   `json:"stderr_truncated,omitempty"`
	StdoutBytes     int64  `json:"stdout_bytes,omitempty"`
	StderrBytes     int64  `json:"stderr_bytes,omitempty"`
}

func (r ToolResult) ModelOutput() string {
	encoded, err := json.Marshal(r)
	if err != nil {
		return `{"exit_code":1,"stderr":"could not encode shell result"}`
	}
	return string(encoded)
}

type Decision struct {
	Text              string     `json:"text,omitempty"`
	ShellCall         *ShellCall `json:"shell_call,omitempty"`
	StopReason        string     `json:"stop_reason,omitempty"`
	ProviderRequestID string     `json:"provider_request_id,omitempty"`
	Usage             Usage      `json:"usage,omitempty"`
}

type Model interface {
	Start(systemPrompt, userPrompt string, tool ToolDefinition) (Conversation, error)
}

type Conversation interface {
	Next(context.Context, *ToolResult) (Decision, error)
	RespondWithoutTools(context.Context, *ToolResult, string) (Decision, error)
}

type Shell interface {
	Run(context.Context, string) (ToolResult, error)
}

type EventSink interface {
	Emit(Event) error
}
