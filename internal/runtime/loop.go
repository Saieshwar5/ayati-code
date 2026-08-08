package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Runtime struct {
	Model        Model
	Provider     string
	ModelName    string
	Shell        Shell
	Sink         EventSink
	Limits       Limits
	Context      ContextPolicy
	SystemPrompt string
	now          func() time.Time
}

type runState struct {
	sequence         int
	steps            int
	toolCalls        int
	modelCalls       int
	contextRollovers int
	finalized        bool
	usage            Usage
}

func (r *Runtime) Run(ctx context.Context, request Request) (Result, error) {
	if err := validateRequest(request); err != nil {
		return Result{}, err
	}
	if r.Model == nil {
		return Result{}, fmt.Errorf("model is required")
	}
	if r.Shell == nil {
		return Result{}, fmt.Errorf("shell is required")
	}
	if err := r.Context.Validate(); err != nil {
		return Result{}, err
	}

	limits := normalizedLimits(r.Limits)
	started := r.clock()()
	state := &runState{}
	runCtx, cancel := context.WithTimeout(ctx, limits.RunTimeout)
	defer cancel()
	if err := r.emit(state, Event{
		Type: EventRunStarted, RunID: request.RunID, Provider: r.Provider, Model: r.ModelName,
		Prompt: request.Prompt, Workspace: request.Workspace, Limits: snapshotLimits(limits, r.Context),
	}); err != nil {
		return Result{}, fmt.Errorf("emit run start: %w", err)
	}

	systemPrompt := r.SystemPrompt
	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = DefaultSystemPrompt
	}
	activePrompt := request.Prompt
	conversation, err := r.Model.Start(systemPrompt, activePrompt, ShellDefinition)
	if err != nil {
		return r.finish(request.RunID, StatusFailed, "", "start model conversation: "+err.Error(), state, started)
	}

	var observation *ToolResult
	var lastResult *ToolResult
	contextEstimate := estimateTextTokens(systemPrompt) + estimateTextTokens(activePrompt) + 512
	checkpointAt := contextCheckpointTokens(r.Context)

	for step := 1; step <= limits.MaxSteps; step++ {
		decisionCtx, stopDecision := context.WithTimeout(runCtx, limits.ModelTimeout)
		state.modelCalls++
		decision, decisionErr := conversation.Next(decisionCtx, observation)
		stopDecision()
		if decisionErr != nil {
			status, reason := contextOutcome(runCtx, "model decision: "+decisionErr.Error())
			return r.finish(request.RunID, status, fallbackHandoff(reason, state, lastResult, decisionErr), reason, state, started)
		}
		state.steps = step
		state.usage.Add(decision.Usage)
		if strings.TrimSpace(decision.Text) == "" && decision.ShellCall == nil {
			reason := "provider returned an empty decision"
			return r.finish(request.RunID, StatusFailed, fallbackHandoff(reason, state, lastResult, nil), reason, state, started)
		}
		if err := r.emit(state, Event{Type: EventModelDecision, RunID: request.RunID, Step: step, Phase: "work", Decision: &decision}); err != nil {
			return Result{}, fmt.Errorf("emit model decision: %w", err)
		}
		contextEstimate += estimateDecisionTokens(decision)
		if decision.ShellCall == nil {
			return r.finish(request.RunID, StatusCompleted, decision.Text, "", state, started)
		}

		command := strings.TrimSpace(decision.ShellCall.Command)
		if command == "" {
			reason := "provider returned an empty shell command"
			return r.finish(request.RunID, StatusFailed, fallbackHandoff(reason, state, lastResult, nil), reason, state, started)
		}
		if err := r.emit(state, Event{Type: EventToolStarted, RunID: request.RunID, Step: step, Phase: "work", Command: command}); err != nil {
			return Result{}, fmt.Errorf("emit tool start: %w", err)
		}
		toolCtx, stopTool := context.WithTimeout(runCtx, limits.ShellTimeout)
		toolResult, toolErr := r.Shell.Run(toolCtx, command)
		stopTool()
		if toolErr != nil {
			status, reason := contextOutcome(runCtx, "execute shell: "+toolErr.Error())
			return r.finish(request.RunID, status, fallbackHandoff(reason, state, lastResult, toolErr), reason, state, started)
		}
		state.toolCalls++
		lastResult = &toolResult
		if err := r.emit(state, Event{Type: EventToolCompleted, RunID: request.RunID, Step: step, Phase: "work", ToolResult: &toolResult}); err != nil {
			return Result{}, fmt.Errorf("emit tool result: %w", err)
		}
		observation = &toolResult
		contextEstimate += estimateTextTokens(toolResult.ModelOutput())
		if runCtx.Err() != nil {
			status, reason := contextOutcome(runCtx, "run stopped")
			return r.finish(request.RunID, status, fallbackHandoff(reason, state, lastResult, runCtx.Err()), reason, state, started)
		}
		if step == limits.MaxSteps {
			return r.finalizeStopped(runCtx, request.RunID, conversation, observation, StatusExhausted, "maximum work steps reached", state, lastResult, started, limits.ModelTimeout)
		}

		pressure := contextPressure(contextEstimate, decision, toolResult)
		if checkpointAt > 0 && pressure >= checkpointAt {
			if state.contextRollovers >= limits.MaxContextRollovers {
				return r.finalizeStopped(runCtx, request.RunID, conversation, observation, StatusExhausted, "maximum context rollovers reached", state, lastResult, started, limits.ModelTimeout)
			}
			conversation, activePrompt, err = r.rolloverContext(
				runCtx, request, systemPrompt, conversation, observation, state, step, limits.ModelTimeout,
			)
			if err != nil {
				var eventErr *eventEmissionError
				if errors.As(err, &eventErr) {
					return Result{}, eventErr
				}
				reason := err.Error()
				return r.finish(request.RunID, StatusFailed, fallbackHandoff(reason, state, lastResult, err), reason, state, started)
			}
			observation = nil
			contextEstimate = estimateTextTokens(systemPrompt) + estimateTextTokens(activePrompt) + 512
		}
	}

	return r.finalizeStopped(runCtx, request.RunID, conversation, observation, StatusExhausted, "maximum work steps reached", state, lastResult, started, limits.ModelTimeout)
}
