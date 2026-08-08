package runtime

import (
	"context"
	"fmt"
	"time"
)

const (
	contextCheckpointPercent = 70
	contextSafetyTokens      = 4096
	defaultMaxOutputTokens   = 8192
)

// Validate checks whether a configured context policy leaves enough room for
// both the model response and the runtime's checkpoint safety reserve. A zero
// window intentionally disables proactive context rollover.
func (p ContextPolicy) Validate() error {
	if p.WindowTokens < 0 {
		return fmt.Errorf("context window tokens cannot be negative")
	}
	if p.MaxOutputTokens < 0 {
		return fmt.Errorf("maximum output tokens cannot be negative")
	}
	if p.WindowTokens == 0 {
		return nil
	}
	maxOutput := p.MaxOutputTokens
	if maxOutput == 0 {
		maxOutput = defaultMaxOutputTokens
	}
	if maxOutput >= p.WindowTokens {
		return fmt.Errorf("maximum output tokens must be smaller than context window tokens")
	}
	if p.WindowTokens-maxOutput <= contextSafetyTokens {
		return fmt.Errorf("context window tokens must leave more than %d tokens beyond maximum output tokens", contextSafetyTokens)
	}
	return nil
}

func contextCheckpointTokens(policy ContextPolicy) int {
	if policy.WindowTokens == 0 {
		return 0
	}
	maxOutput := policy.MaxOutputTokens
	if maxOutput == 0 {
		maxOutput = defaultMaxOutputTokens
	}
	usable := policy.WindowTokens - maxOutput - contextSafetyTokens
	return usable * contextCheckpointPercent / 100
}

func contextPressure(estimated int, decision Decision, result ToolResult) int {
	if decision.Usage.InputTokens == 0 {
		return estimated
	}
	reported := int(decision.Usage.InputTokens) + estimateDecisionTokens(decision) + estimateTextTokens(result.ModelOutput())
	if reported > estimated {
		return reported
	}
	return estimated
}

func (r *Runtime) rolloverContext(
	ctx context.Context,
	request Request,
	systemPrompt string,
	conversation Conversation,
	observation *ToolResult,
	state *runState,
	step int,
	modelTimeout time.Duration,
) (Conversation, string, error) {
	checkpointCtx, cancel := context.WithTimeout(ctx, modelTimeout)
	state.modelCalls++
	checkpoint, checkpointErr := conversation.RespondWithoutTools(checkpointCtx, observation, CheckpointPrompt)
	cancel()
	if err := validateToolDisabledResponse(checkpoint, checkpointErr); err != nil {
		return nil, "", fmt.Errorf("context checkpoint failed: %w", err)
	}

	state.usage.Add(checkpoint.Usage)
	state.contextRollovers++
	if err := r.emit(state, Event{
		Type: EventContextCheckpoint, RunID: request.RunID, Step: step, Phase: "checkpoint",
		ContextRollover: state.contextRollovers, Decision: &checkpoint,
	}); err != nil {
		return nil, "", &eventEmissionError{operation: "emit context checkpoint", cause: err}
	}

	continuationPrompt := ContinuationPrompt(request.Prompt, checkpoint.Text)
	nextConversation, err := r.Model.Start(systemPrompt, continuationPrompt, ShellDefinition)
	if err != nil {
		return nil, "", fmt.Errorf("start context continuation: %w", err)
	}
	return nextConversation, continuationPrompt, nil
}

func estimateTextTokens(value string) int {
	return (len(value) + 2) / 3
}

func estimateDecisionTokens(decision Decision) int {
	total := estimateTextTokens(decision.Text) + 128
	if decision.ShellCall != nil {
		total += estimateTextTokens(decision.ShellCall.Command)
	}
	return total
}
