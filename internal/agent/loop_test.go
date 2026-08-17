package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type scriptedProvider struct {
	messages []Message
	calls    int
}

func (p *scriptedProvider) Next(_ context.Context, _ Request) (Message, error) {
	p.calls++
	if len(p.messages) == 0 {
		return Message{}, fmt.Errorf("no scripted message")
	}
	message := p.messages[0]
	p.messages = p.messages[1:]
	return message, nil
}

type memoryRecorder struct{ messages []Message }

func (r *memoryRecorder) Append(message Message) error {
	r.messages = append(r.messages, message)
	return nil
}

type fixedShell struct{ calls int }

func (s *fixedShell) Execute(_ context.Context, request ShellRequest) ShellResult {
	s.calls++
	return ShellResult{Command: request.Command, ExitCode: 0, Stdout: "ok"}
}

func TestLoopCompletesAfterOneFinalResponse(t *testing.T) {
	provider := &scriptedProvider{messages: []Message{{Role: "assistant", Content: "done"}}}
	recorder := &memoryRecorder{}
	history := []Message{}
	completion, err := (Loop{
		Provider: provider, Shell: &fixedShell{}, Recorder: recorder, Model: "test",
	}).Run(context.Background(), &history, "fix it")
	if err != nil || completion.Text != "done" || completion.Steps != 1 {
		t.Fatalf("completion = %#v, error = %v", completion, err)
	}
	if len(history) != 2 || len(recorder.messages) != 2 {
		t.Fatalf("history = %#v", history)
	}
}

func TestLoopContinuesAlreadyRecordedUserRequest(t *testing.T) {
	provider := &scriptedProvider{messages: []Message{{Role: "assistant", Content: "done"}}}
	recorder := &memoryRecorder{}
	history := []Message{{Role: "user", Content: "durable request"}}
	completion, err := (Loop{
		Provider: provider, Recorder: recorder, Model: "test",
	}).Continue(context.Background(), &history)
	if err != nil || completion.Text != "done" {
		t.Fatalf("completion = %#v, error = %v", completion, err)
	}
	if len(history) != 2 || len(recorder.messages) != 1 || recorder.messages[0].Role != "assistant" {
		t.Fatalf("history = %#v, recorded = %#v", history, recorder.messages)
	}
}

func TestLoopStopsExactlyAtTwentyDecisions(t *testing.T) {
	messages := make([]Message, MaxSteps)
	for index := range messages {
		messages[index] = Message{Role: "assistant", ToolCalls: []ToolCall{{
			ID: fmt.Sprintf("call-%d", index), Type: "function",
			Function: FunctionCall{Name: "shell", Arguments: `{"command":"true"}`},
		}}}
	}
	provider := &scriptedProvider{messages: messages}
	shell := &fixedShell{}
	history := []Message{}
	completion, err := (Loop{
		Provider: provider, Shell: shell, Recorder: &memoryRecorder{}, Model: "test",
	}).Run(context.Background(), &history, "work")
	if !errors.Is(err, ErrStepLimit) || !completion.Exhausted {
		t.Fatalf("completion = %#v, error = %v", completion, err)
	}
	if provider.calls != MaxSteps || shell.calls != MaxSteps {
		t.Fatalf("provider calls = %d, shell calls = %d", provider.calls, shell.calls)
	}
}

func TestLoopHonorsCustomStepLimit(t *testing.T) {
	messages := make([]Message, 3)
	for index := range messages {
		messages[index] = Message{Role: "assistant", ToolCalls: []ToolCall{{
			ID: fmt.Sprintf("call-%d", index), Type: "function",
			Function: FunctionCall{Name: "shell", Arguments: `{"command":"true"}`},
		}}}
	}
	provider := &scriptedProvider{messages: messages}
	history := []Message{}
	completion, err := (Loop{
		Provider: provider, Shell: &fixedShell{}, Recorder: &memoryRecorder{},
		Model: "test", StepLimit: 3,
	}).Run(context.Background(), &history, "work")
	if !errors.Is(err, ErrStepLimit) || completion.Steps != 3 || !completion.Exhausted {
		t.Fatalf("completion = %#v, error = %v", completion, err)
	}
}

func TestLoopRejectsShellCallWhenCapabilityIsDisabled(t *testing.T) {
	provider := &scriptedProvider{messages: []Message{
		{Role: "assistant", ToolCalls: []ToolCall{{
			ID: "call-1", Type: "function",
			Function: FunctionCall{Name: "shell", Arguments: `{"command":"pwd"}`},
		}}},
	}}
	history := []Message{}
	_, err := (Loop{
		Provider: provider, Recorder: &memoryRecorder{}, Model: "test",
	}).Run(context.Background(), &history, "inspect")
	if err == nil || err.Error() != "provider returned a shell call for an agent without shell capability" {
		t.Fatalf("error = %v", err)
	}
}

func TestParseShellCallRequiresCommand(t *testing.T) {
	_, err := parseShellCall(ToolCall{
		ID: "call-1", Type: "function",
		Function: FunctionCall{Name: "shell", Arguments: `{}`},
	})
	if err == nil || err.Error() != "shell command is empty" {
		t.Fatalf("error = %v", err)
	}
}
