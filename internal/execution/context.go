package execution

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Saieshwar5/perpetual/internal/workspace"
)

// Context is a bounded, assembled view of one execution room at one step. It
// is rebuilt for every model call and never mutates durable state.
type Context struct {
	// System is the versioned identity/tool prompt.
	System string
	// Messages are the visible, bounded conversation lines for the provider.
	Messages []string
}

// WithSummary prepends a compaction summary to the assembled messages so the
// model sees condensed history before recent steps.
func (c Context) WithSummary(summary string) Context {
	messages := make([]string, 0, len(c.Messages)+2)
	if strings.TrimSpace(summary) != "" {
		messages = append(messages, "<summary>", summary, "</summary>")
	}
	messages = append(messages, c.Messages...)
	return Context{System: c.System, Messages: messages}
}

// TokenCount returns the estimated prompt tokens for this context.
func (c Context) TokenCount() int64 {
	total := EstimateTokens(c.System)
	for _, message := range c.Messages {
		total += EstimateTokens(message)
	}
	return total
}

// BuildContext assembles the context snapshot for one run from durable state.
func BuildContext(ctx context.Context, store *workspace.Store, run workspace.Run) (Context, error) {
	if store == nil {
		return Context{}, errors.New("execution context store is required")
	}
	steps, err := store.RunSteps(ctx, run.ID)
	if err != nil {
		return Context{}, err
	}
	var memory map[string]any
	if stored, err := store.WorkMemory(ctx, run.ID); err == nil {
		memory = stored.Notes
	} else {
		// Missing memory on a fresh run is not fatal.
		memory = map[string]any{}
	}
	workspaceSummary := ""
	if ws, err := store.Get(ctx, run.WorkspaceID); err == nil {
		workspaceSummary = "Workspace: " + ws.Repository + " | Branch: " + ws.Branch
	}

	messages := make([]string, 0, len(steps)+3)
	if workspaceSummary != "" {
		messages = append(messages, workspaceSummary)
	}
	if len(memory) > 0 {
		encoded, err := json.Marshal(memory)
		if err == nil {
			messages = append(messages, "Memory: "+string(encoded))
		}
	}
	for _, step := range steps {
		messages = append(messages, RenderStep(step))
	}
	if len(messages) == 0 {
		messages = []string{"Fresh execution room. No steps have completed yet."}
	}
	return Context{System: systemPrompt, Messages: messages}, nil
}

// RenderStep converts one durable step into a model-visible line.
func RenderStep(step workspace.RunStep) string {
	switch step.Kind {
	case workspace.StepShell:
		command, _ := step.Input["command"].(string)
		stdout, _ := step.Output["stdout"].(string)
		exitCode, _ := step.Output["exit_code"].(float64)
		return "shell(" + command + ") -> exit " + formatExit(exitCode) + " stdout: " + stdout
	case workspace.StepCompact:
		summary, _ := step.Output["summary"].(string)
		return "context compaction: " + summary
	default:
		content, _ := step.Output["content"].(string)
		return "model: " + content
	}
}
