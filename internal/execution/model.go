// Package execution owns the execution-room worker loop: it claims durable
// runs, drives the model provider, executes the single shell tool, and records
// idempotent steps so runs resume safely across worker restarts.
package execution

import (
	"context"
)

// ToolCall is one tool invocation requested by the model. Perpetual exposes a
// single "shell" tool; external capabilities ride on CLIs inside the runtime.
type ToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// TokenUsage records provider-reported token accounting.
type TokenUsage struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
	Total  int64 `json:"total"`
}

// ModelRequest is one bounded model call assembled from the context.
type ModelRequest struct {
	System    string
	Messages  []string
	ModelID   string
	MaxTokens int64
	Tools     []string
}

// ModelResponse is the provider result for one call.
type ModelResponse struct {
	Content    string
	StopReason string // stop | length | error | aborted
	ToolCalls  []ToolCall
	Usage      TokenUsage
}

// ModelProvider is the controller-side seam for chat/coding models. Tests use
// fake implementations; providers never see workspace credentials.
type ModelProvider interface {
	Complete(ctx context.Context, req ModelRequest) (ModelResponse, error)
}
