package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Saieshwar5/perpetual/internal/workspace"
)

// Settings controls pi-style context compaction.
type Settings struct {
	Enabled          bool
	ReserveTokens    int64
	KeepRecentTokens int64
}

// DefaultSettings matches pi's defaults: compact only when the prompt is near
// the model window edge and keep a recent token budget intact.
func DefaultSettings() Settings {
	return Settings{Enabled: true, ReserveTokens: 16384, KeepRecentTokens: 20000}
}

// ShouldCompact reports whether the context should be compacted.
func ShouldCompact(contextTokens, contextWindow int64, settings Settings) bool {
	if !settings.Enabled || contextWindow <= 0 {
		return false
	}
	return contextTokens > contextWindow-settings.ReserveTokens
}

// CutSteps splits durable steps into a summarized (older) prefix and a kept
// (recent) suffix. Tool results are never split from their model turn.
func CutSteps(steps []workspace.RunStep, keepRecentTokens int64) (kept, summarized []workspace.RunStep) {
	if len(steps) == 0 {
		return steps, nil
	}
	var accumulated int64
	cutAt := 0
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		accumulated += EstimateTokens(RenderStep(step))
		if step.Kind == workspace.StepShell {
			// Tool results must stay with the recent side; keep walking older.
			continue
		}
		if accumulated >= keepRecentTokens {
			cutAt = i
			break
		}
	}
	// Never cut at a shell/model boundary deeper than the newest turn: if we
	// walked off the front, everything is recent.
	return steps[cutAt:], steps[:cutAt]
}

// Compactor generates structured history summaries through the model provider.
type Compactor struct {
	model    ModelProvider
	settings Settings
}

// NewCompactor builds a Compactor.
func NewCompactor(model ModelProvider, settings Settings) (*Compactor, error) {
	if model == nil {
		return nil, errors.New("compactor model provider is required")
	}
	return &Compactor{model: model, settings: settings}, nil
}

// Compact summarizes the given older steps and returns the structured summary.
func (c *Compactor) Compact(ctx context.Context, toSummarize []workspace.RunStep, previousSummary string) (string, error) {
	var history strings.Builder
	for _, step := range toSummarize {
		history.WriteString(RenderStep(step))
		history.WriteString("\n")
	}
	promptText := history.String()
	if strings.TrimSpace(previousSummary) != "" {
		promptText += fmt.Sprintf(updateSummarizationPrompt, previousSummary)
	} else {
		promptText += summarizationPrompt
	}
	response, err := c.model.Complete(ctx, ModelRequest{
		Messages:  []string{promptText},
		MaxTokens: c.settings.ReserveTokens,
	})
	if err != nil {
		return "", err
	}
	if response.StopReason == "error" || response.StopReason == "aborted" {
		return "", errors.New("summarization failed: " + response.Content)
	}
	return response.Content, nil
}
