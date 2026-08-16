package runtime

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRunCompletesWithoutTool(t *testing.T) {
	conversation := &scriptedConversation{decisions: []Decision{{Text: "done", Usage: Usage{InputTokens: 10, OutputTokens: 2, TotalTokens: 12}}}}
	sink := &eventRecorder{}
	runner := Runtime{Model: scriptedModel{conversation}, Shell: &fakeShell{}, Sink: sink, Limits: shortLimits()}

	result, err := runner.Run(context.Background(), testRequest(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != StatusCompleted || result.Final != "done" || result.Steps != 1 || result.ToolCalls != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Usage.TotalTokens != 12 {
		t.Fatalf("usage = %+v", result.Usage)
	}
	wantTypes := []string{EventRunStarted, EventModelDecision, EventRunCompleted}
	if got := eventTypes(sink.events); !reflect.DeepEqual(got, wantTypes) {
		t.Fatalf("event types = %v, want %v", got, wantTypes)
	}
	assertEventSequence(t, sink.events)
}

func TestRunFeedsExactlyOneShellResultBackToModel(t *testing.T) {
	conversation := &scriptedConversation{decisions: []Decision{
		{ShellCall: &ShellCall{Command: "go test ./..."}, Usage: Usage{InputTokens: 10}},
		{Text: "tests pass", Usage: Usage{OutputTokens: 3}},
	}}
	shell := &fakeShell{results: []ToolResult{{Stdout: "ok\n", ExitCode: 0, DurationMS: 20}}}
	sink := &eventRecorder{}
	runner := Runtime{Model: scriptedModel{conversation}, Shell: shell, Sink: sink, Limits: shortLimits()}

	result, err := runner.Run(context.Background(), testRequest(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != StatusCompleted || result.ToolCalls != 1 || result.Steps != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !reflect.DeepEqual(shell.commands, []string{"go test ./..."}) {
		t.Fatalf("commands = %v", shell.commands)
	}
	if len(conversation.observations) != 2 || conversation.observations[0] != nil || conversation.observations[1] == nil || conversation.observations[1].Stdout != "ok\n" {
		t.Fatalf("observations = %+v", conversation.observations)
	}
	wantTypes := []string{EventRunStarted, EventModelDecision, EventToolStarted, EventToolCompleted, EventModelDecision, EventRunCompleted}
	if got := eventTypes(sink.events); !reflect.DeepEqual(got, wantTypes) {
		t.Fatalf("event types = %v, want %v", got, wantTypes)
	}
}

func TestRunExhaustsOnlyAfterRecordingLastToolResult(t *testing.T) {
	conversation := &scriptedConversation{
		decisions:            []Decision{{ShellCall: &ShellCall{Command: "touch result"}}},
		maintenanceDecisions: []Decision{{Text: "The file was created; no verification was run.", Usage: Usage{TotalTokens: 7}}},
	}
	shell := &fakeShell{results: []ToolResult{{ExitCode: 0}}}
	sink := &eventRecorder{}
	limits := shortLimits()
	limits.MaxSteps = 1
	runner := Runtime{Model: scriptedModel{conversation}, Shell: shell, Sink: sink, Limits: limits}

	result, err := runner.Run(context.Background(), testRequest(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != StatusExhausted || result.ToolCalls != 1 || conversation.calls != 1 || result.Final == "" || !result.Finalized || result.ModelCalls != 2 {
		t.Fatalf("unexpected result: %+v, model calls %d", result, conversation.calls)
	}
	if conversation.maintenanceCalls != 1 || len(conversation.maintenanceObservations) != 1 || conversation.maintenanceObservations[0] == nil {
		t.Fatalf("finalization input was not the last shell result: %+v", conversation.maintenanceObservations)
	}
	wantTypes := []string{EventRunStarted, EventModelDecision, EventToolStarted, EventToolCompleted, EventRunFinalizing, EventModelDecision, EventRunExhausted}
	if got := eventTypes(sink.events); !reflect.DeepEqual(got, wantTypes) {
		t.Fatalf("event types = %v, want %v", got, wantTypes)
	}
}

func TestRunFallsBackWhenExhaustionFinalizationFails(t *testing.T) {
	conversation := &scriptedConversation{
		decisions:         []Decision{{ShellCall: &ShellCall{Command: "true"}}},
		maintenanceErrors: []error{errors.New("provider unavailable")},
	}
	limits := shortLimits()
	limits.MaxSteps = 1
	runner := Runtime{Model: scriptedModel{conversation}, Shell: &fakeShell{results: []ToolResult{{ExitCode: 0}}}, Limits: limits}

	result, err := runner.Run(context.Background(), testRequest(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != StatusExhausted || result.Finalized || !strings.Contains(result.Final, "provider unavailable") {
		t.Fatalf("unexpected fallback: %+v", result)
	}
}

func TestRunCheckpointsAndStartsFreshContext(t *testing.T) {
	first := &scriptedConversation{
		decisions:            []Decision{{ShellCall: &ShellCall{Command: "printf changed"}, Usage: Usage{InputTokens: 4000}}},
		maintenanceDecisions: []Decision{{Text: "Changed the target; verification remains.", Usage: Usage{TotalTokens: 40}}},
	}
	second := &scriptedConversation{decisions: []Decision{{Text: "verified and complete", Usage: Usage{TotalTokens: 20}}}}
	model := &queuedModel{conversations: []Conversation{first, second}}
	sink := &eventRecorder{}
	runner := Runtime{
		Model: model, Shell: &fakeShell{results: []ToolResult{{Stdout: "changed", ExitCode: 0}}}, Sink: sink,
		Limits: shortLimits(), Context: ContextPolicy{WindowTokens: 10_000, MaxOutputTokens: 1_000},
	}

	result, err := runner.Run(context.Background(), testRequest(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != StatusCompleted || result.ContextRollovers != 1 || result.ModelCalls != 3 || result.Finalized {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(model.userPrompts) != 2 || !strings.Contains(model.userPrompts[1], "do the task") || !strings.Contains(model.userPrompts[1], "Changed the target") {
		t.Fatalf("continuation did not preserve request and checkpoint: %#v", model.userPrompts)
	}
	if first.maintenanceCalls != 1 || len(first.maintenanceInstructions) != 1 || !strings.Contains(first.maintenanceInstructions[0], "checkpoint") {
		t.Fatalf("checkpoint call not recorded: %+v", first.maintenanceInstructions)
	}
	wantTypes := []string{EventRunStarted, EventModelDecision, EventToolStarted, EventToolCompleted, EventContextCheckpoint, EventModelDecision, EventRunCompleted}
	if got := eventTypes(sink.events); !reflect.DeepEqual(got, wantTypes) {
		t.Fatalf("event types = %v, want %v", got, wantTypes)
	}
	if sink.events[0].Limits == nil || sink.events[0].Limits.ContextCheckpointTokens == 0 {
		t.Fatalf("context policy missing from start event: %+v", sink.events[0].Limits)
	}
}

func TestRunFinalizesWhenContextRolloverLimitIsReached(t *testing.T) {
	first := &scriptedConversation{
		decisions:            []Decision{{ShellCall: &ShellCall{Command: "printf first"}, Usage: Usage{InputTokens: 4000}}},
		maintenanceDecisions: []Decision{{Text: "first checkpoint"}},
	}
	second := &scriptedConversation{
		decisions:            []Decision{{ShellCall: &ShellCall{Command: "printf second"}, Usage: Usage{InputTokens: 4000}}},
		maintenanceDecisions: []Decision{{Text: "Stopped after the configured rollover limit."}},
	}
	model := &queuedModel{conversations: []Conversation{first, second}}
	limits := shortLimits()
	limits.MaxContextRollovers = 1
	runner := Runtime{
		Model: model, Shell: &fakeShell{results: []ToolResult{{ExitCode: 0}, {ExitCode: 0}}},
		Limits: limits, Context: ContextPolicy{WindowTokens: 10_000, MaxOutputTokens: 1_000},
	}

	result, err := runner.Run(context.Background(), testRequest(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != StatusExhausted || result.Reason != "maximum context rollovers reached" || result.ContextRollovers != 1 || !result.Finalized || result.ModelCalls != 4 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if second.maintenanceCalls != 1 || !strings.Contains(second.maintenanceInstructions[0], "maximum context rollovers reached") {
		t.Fatalf("missing rollover stop reason: %#v", second.maintenanceInstructions)
	}
}

func TestRunFailsOnEmptyDecision(t *testing.T) {
	conversation := &scriptedConversation{decisions: []Decision{{}}}
	sink := &eventRecorder{}
	runner := Runtime{Model: scriptedModel{conversation}, Shell: &fakeShell{}, Sink: sink, Limits: shortLimits()}

	result, err := runner.Run(context.Background(), testRequest(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != StatusFailed || result.Reason != "provider returned an empty decision" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if got := eventTypes(sink.events); !reflect.DeepEqual(got, []string{EventRunStarted, EventRunFailed}) {
		t.Fatalf("event types = %v", got)
	}
}

func TestRunRejectsRelativeWorkspaceBeforeStarting(t *testing.T) {
	runner := Runtime{}
	_, err := runner.Run(context.Background(), Request{RunID: "run", Prompt: "task", Workspace: "relative"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRunReportsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sink := &eventRecorder{}
	runner := Runtime{Model: scriptedModel{&scriptedConversation{errors: []error{context.Canceled}}}, Shell: &fakeShell{}, Sink: sink, Limits: shortLimits()}
	result, err := runner.Run(ctx, testRequest(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != StatusCancelled || result.Reason != "run cancelled" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if got := eventTypes(sink.events); !reflect.DeepEqual(got, []string{EventRunStarted, EventRunCancelled}) {
		t.Fatalf("event types = %v", got)
	}
}

func TestRunStopsWhenEventJournalFails(t *testing.T) {
	conversation := &scriptedConversation{decisions: []Decision{{ShellCall: &ShellCall{Command: "touch should-not-run"}}}}
	shell := &fakeShell{}
	sink := &failingSink{failAt: 2}
	runner := Runtime{Model: scriptedModel{conversation}, Shell: shell, Sink: sink, Limits: shortLimits()}
	_, err := runner.Run(context.Background(), testRequest(t))
	if err == nil || !strings.Contains(err.Error(), "journal unavailable") {
		t.Fatalf("expected sink error, got %v", err)
	}
	if len(shell.commands) != 0 {
		t.Fatalf("shell ran after journal failure: %v", shell.commands)
	}
}

func TestRunStopsWhenContextCheckpointCannotBeJournaled(t *testing.T) {
	conversation := &scriptedConversation{
		decisions:            []Decision{{ShellCall: &ShellCall{Command: "true"}, Usage: Usage{InputTokens: 4000}}},
		maintenanceDecisions: []Decision{{Text: "checkpoint"}},
	}
	sink := &failingSink{failAt: 5}
	runner := Runtime{
		Model: scriptedModel{conversation}, Shell: &fakeShell{results: []ToolResult{{ExitCode: 0}}}, Sink: sink,
		Limits: shortLimits(), Context: ContextPolicy{WindowTokens: 10_000, MaxOutputTokens: 1_000},
	}

	_, err := runner.Run(context.Background(), testRequest(t))
	if err == nil || !strings.Contains(err.Error(), "emit context checkpoint: journal unavailable") {
		t.Fatalf("expected checkpoint journal error, got %v", err)
	}
	if sink.calls != 5 {
		t.Fatalf("terminal event emitted after checkpoint journal failure: %d calls", sink.calls)
	}
}

func TestRunBoundsEachModelDecision(t *testing.T) {
	limits := shortLimits()
	limits.ModelTimeout = 20 * time.Millisecond
	sink := &eventRecorder{}
	runner := Runtime{Model: scriptedModel{blockingConversation{}}, Shell: &fakeShell{}, Sink: sink, Limits: limits}
	result, err := runner.Run(context.Background(), testRequest(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != StatusFailed || !strings.Contains(result.Reason, "context deadline exceeded") {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func testRequest(t *testing.T) Request {
	t.Helper()
	workspace, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return Request{RunID: "run-test", Prompt: "do the task", Workspace: workspace}
}

func shortLimits() Limits {
	return Limits{MaxSteps: 4, RunTimeout: time.Second, ModelTimeout: time.Second, ShellTimeout: time.Second, MaxOutputBytes: 1024}
}

func eventTypes(events []Event) []string {
	result := make([]string, 0, len(events))
	for _, event := range events {
		result = append(result, event.Type)
	}
	return result
}

func assertEventSequence(t *testing.T, events []Event) {
	t.Helper()
	for index, event := range events {
		if event.Version != ProtocolVersion {
			t.Fatalf("event %d version = %d", index, event.Version)
		}
		if event.Sequence != index+1 {
			t.Fatalf("event %d sequence = %d", index, event.Sequence)
		}
		if event.Timestamp.IsZero() {
			t.Fatalf("event %d has zero timestamp", index)
		}
	}
}
