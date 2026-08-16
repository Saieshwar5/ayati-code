package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type eventEmissionError struct {
	operation string
	cause     error
}

func (e *eventEmissionError) Error() string {
	return fmt.Sprintf("%s: %v", e.operation, e.cause)
}

func (e *eventEmissionError) Unwrap() error {
	return e.cause
}

func (r *Runtime) finalizeStopped(ctx context.Context, runID string, conversation Conversation, observation *ToolResult, status Status, reason string, state *runState, lastResult *ToolResult, started time.Time, timeout time.Duration) (Result, error) {
	if err := r.emit(state, Event{Type: EventRunFinalizing, RunID: runID, Step: state.steps, Phase: "finalization"}); err != nil {
		return Result{}, fmt.Errorf("emit finalization start: %w", err)
	}
	if ctx.Err() != nil {
		return r.finish(runID, status, fallbackHandoff(reason, state, lastResult, ctx.Err()), reason, state, started)
	}
	finalCtx, stopFinal := context.WithTimeout(ctx, timeout)
	state.modelCalls++
	decision, err := conversation.RespondWithoutTools(finalCtx, observation, FinalizationPrompt+"\n\nRuntime stop reason: "+reason)
	stopFinal()
	if finalErr := validateToolDisabledResponse(decision, err); finalErr != nil {
		return r.finish(runID, status, fallbackHandoff(reason, state, lastResult, finalErr), reason, state, started)
	}
	state.usage.Add(decision.Usage)
	state.finalized = true
	if err := r.emit(state, Event{Type: EventModelDecision, RunID: runID, Step: state.steps, Phase: "finalization", Decision: &decision}); err != nil {
		return Result{}, fmt.Errorf("emit finalization decision: %w", err)
	}
	return r.finish(runID, status, decision.Text, reason, state, started)
}

func validateToolDisabledResponse(decision Decision, responseErr error) error {
	if responseErr != nil {
		return responseErr
	}
	if decision.ShellCall != nil {
		return fmt.Errorf("provider returned a shell call while tools were disabled")
	}
	if strings.TrimSpace(decision.Text) == "" {
		return fmt.Errorf("provider returned an empty response while tools were disabled")
	}
	return nil
}

func (r *Runtime) finish(runID string, status Status, final, reason string, state *runState, started time.Time) (Result, error) {
	result := Result{
		RunID: runID, Provider: r.Provider, Model: r.ModelName,
		Status: status, Final: final, Reason: reason,
		Steps: state.steps, ToolCalls: state.toolCalls, ModelCalls: state.modelCalls,
		ContextRollovers: state.contextRollovers, Finalized: state.finalized,
		Usage: state.usage, DurationMS: r.clock()().Sub(started).Milliseconds(),
	}
	eventType := terminalEventType(status)
	if err := r.emit(state, Event{Type: eventType, RunID: runID, Outcome: &result}); err != nil {
		return result, fmt.Errorf("emit terminal event: %w", err)
	}
	return result, nil
}

func terminalEventType(status Status) string {
	switch status {
	case StatusCompleted:
		return EventRunCompleted
	case StatusExhausted:
		return EventRunExhausted
	case StatusCancelled:
		return EventRunCancelled
	default:
		return EventRunFailed
	}
}

func (r *Runtime) emit(state *runState, event Event) error {
	if r.Sink == nil {
		return nil
	}
	state.sequence++
	event.Version = ProtocolVersion
	event.Sequence = state.sequence
	event.Timestamp = r.clock()().UTC()
	return r.Sink.Emit(event)
}

func (r *Runtime) clock() func() time.Time {
	if r.now != nil {
		return r.now
	}
	return time.Now
}

func fallbackHandoff(reason string, state *runState, lastResult *ToolResult, finalErr error) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Run stopped: %s. Completed %d work steps and %d shell calls.", reason, state.steps, state.toolCalls)
	if lastResult != nil {
		fmt.Fprintf(&builder, " Last shell result: exit code %d", lastResult.ExitCode)
		if lastResult.TimedOut {
			builder.WriteString(", timed out")
		}
		builder.WriteString(". Exact output is preserved in the JSONL event stream.")
	}
	if finalErr != nil {
		fmt.Fprintf(&builder, " A final model handoff was unavailable: %v.", finalErr)
	}
	return builder.String()
}

func contextOutcome(runCtx context.Context, fallback string) (Status, string) {
	switch runCtx.Err() {
	case context.Canceled:
		return StatusCancelled, "run cancelled"
	case context.DeadlineExceeded:
		return StatusExhausted, "run timeout reached"
	default:
		return StatusFailed, fallback
	}
}
