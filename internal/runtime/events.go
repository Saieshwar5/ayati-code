package runtime

import "time"

type Status string

const (
	StatusCompleted Status = "completed"
	StatusExhausted Status = "exhausted"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

const (
	EventRunStarted        = "run.started"
	EventModelDecision     = "model.decision"
	EventToolStarted       = "tool.started"
	EventToolCompleted     = "tool.completed"
	EventContextCheckpoint = "context.checkpoint"
	EventRunFinalizing     = "run.finalizing"
	EventRunCompleted      = "run.completed"
	EventRunExhausted      = "run.exhausted"
	EventRunFailed         = "run.failed"
	EventRunCancelled      = "run.cancelled"
)

type Event struct {
	Version         int            `json:"version"`
	Sequence        int            `json:"seq"`
	Timestamp       time.Time      `json:"timestamp"`
	Type            string         `json:"type"`
	RunID           string         `json:"run_id"`
	Provider        string         `json:"provider,omitempty"`
	Model           string         `json:"model,omitempty"`
	Prompt          string         `json:"prompt,omitempty"`
	Workspace       string         `json:"workspace,omitempty"`
	Limits          *LimitSnapshot `json:"limits,omitempty"`
	Phase           string         `json:"phase,omitempty"`
	ContextRollover int            `json:"context_rollover,omitempty"`
	Step            int            `json:"step,omitempty"`
	Text            string         `json:"text,omitempty"`
	Command         string         `json:"command,omitempty"`
	Decision        *Decision      `json:"decision,omitempty"`
	ToolResult      *ToolResult    `json:"tool_result,omitempty"`
	Outcome         *Result        `json:"outcome,omitempty"`
}

type Result struct {
	RunID            string `json:"run_id"`
	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
	Status           Status `json:"status"`
	Final            string `json:"final,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Steps            int    `json:"steps"`
	ToolCalls        int    `json:"tool_calls"`
	ModelCalls       int    `json:"model_calls"`
	ContextRollovers int    `json:"context_rollovers"`
	Finalized        bool   `json:"finalized"`
	Usage            Usage  `json:"usage"`
	DurationMS       int64  `json:"duration_ms"`
}
