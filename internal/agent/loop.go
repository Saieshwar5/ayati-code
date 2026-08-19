package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrStepLimit = errors.New("agent reached its step limit")

type shellInvocation struct {
	Request ShellRequest
}

type Loop struct {
	Provider  Provider
	Shell     Shell
	Recorder  Recorder
	Observer  Observer
	Model     string
	Prompt    string
	StepLimit int
}

func (l Loop) Run(ctx context.Context, history *[]Message, userText string) (Completion, error) {
	stepLimit, err := l.validate(ctx, history)
	if err != nil {
		return Completion{}, err
	}
	if strings.TrimSpace(userText) == "" {
		return Completion{}, fmt.Errorf("model and user request are required")
	}
	user := Message{Role: "user", Content: userText}
	if err := l.record(history, user); err != nil {
		return Completion{}, err
	}
	return l.continueRun(ctx, history, stepLimit)
}

// Continue resumes a run whose user request has already been durably recorded.
func (l Loop) Continue(ctx context.Context, history *[]Message) (Completion, error) {
	stepLimit, err := l.validate(ctx, history)
	if err != nil {
		return Completion{}, err
	}
	if len(*history) == 0 || (*history)[len(*history)-1].Role != "user" {
		return Completion{}, fmt.Errorf("recorded history must end with a user request")
	}
	return l.continueRun(ctx, history, stepLimit)
}

func (l Loop) validate(ctx context.Context, history *[]Message) (int, error) {
	if l.Provider == nil || l.Recorder == nil || history == nil {
		return 0, fmt.Errorf("agent loop is not fully configured")
	}
	if strings.TrimSpace(l.Model) == "" {
		return 0, fmt.Errorf("model is required")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	stepLimit := l.StepLimit
	if stepLimit == 0 {
		stepLimit = MaxSteps
	}
	if stepLimit < 1 || stepLimit > MaxSteps {
		return 0, fmt.Errorf("agent step limit must be between 1 and %d", MaxSteps)
	}
	return stepLimit, nil
}

func (l Loop) continueRun(ctx context.Context, history *[]Message, stepLimit int) (Completion, error) {
	for step := 1; step <= stepLimit; step++ {
		if err := ctx.Err(); err != nil {
			return Completion{Steps: step - 1}, err
		}
		if l.Observer != nil {
			l.Observer.Step(step, stepLimit)
		}
		prompt := strings.TrimSpace(l.Prompt)
		if prompt == "" {
			prompt = SystemPrompt
		}
		message, err := l.Provider.Next(ctx, Request{
			Model: l.Model, SystemPrompt: prompt, Messages: append([]Message(nil), (*history)...),
			DisableShell: l.Shell == nil,
		})
		if err != nil {
			return Completion{Steps: step}, err
		}
		if message.Role == "" {
			message.Role = "assistant"
		}
		if message.Role != "assistant" {
			return Completion{Steps: step}, fmt.Errorf("provider returned role %q", message.Role)
		}
		if len(message.ToolCalls) > 1 {
			return Completion{Steps: step}, fmt.Errorf("provider returned %d tool calls; Perpetual permits one", len(message.ToolCalls))
		}
		if len(message.ToolCalls) == 0 {
			if strings.TrimSpace(message.Content) == "" {
				return Completion{Steps: step}, fmt.Errorf("provider returned an empty response")
			}
			if err := l.record(history, message); err != nil {
				return Completion{Steps: step}, err
			}
			if l.Observer != nil {
				l.Observer.Assistant(message.Content)
			}
			return Completion{Text: message.Content, Steps: step}, nil
		}
		call := message.ToolCalls[0]
		if l.Shell == nil {
			return Completion{Steps: step}, fmt.Errorf("provider returned a shell call for an agent without shell capability")
		}
		invocation, err := parseShellCall(call)
		if err != nil {
			return Completion{Steps: step}, err
		}
		if err := l.record(history, message); err != nil {
			return Completion{Steps: step}, err
		}
		if l.Observer != nil {
			l.Observer.ToolCall(invocation.Request)
		}
		result := l.Shell.Execute(ctx, invocation.Request)
		encoded, err := json.Marshal(result)
		if err != nil {
			return Completion{Steps: step}, fmt.Errorf("encode shell result: %w", err)
		}
		tool := Message{Role: "tool", ToolCallID: call.ID, Content: string(encoded)}
		if err := l.record(history, tool); err != nil {
			return Completion{Steps: step}, err
		}
		if l.Observer != nil {
			l.Observer.ToolResult(result)
		}
		if err := ctx.Err(); err != nil {
			return Completion{Steps: step}, err
		}
	}
	return Completion{Steps: stepLimit, Exhausted: true}, fmt.Errorf("%w: %d steps", ErrStepLimit, stepLimit)
}

func (l Loop) record(history *[]Message, message Message) error {
	if err := l.Recorder.Append(message); err != nil {
		return fmt.Errorf("record %s message: %w", message.Role, err)
	}
	*history = append(*history, message)
	return nil
}

func parseShellCall(call ToolCall) (shellInvocation, error) {
	if call.ID == "" || call.Type != "function" || call.Function.Name != "shell" {
		return shellInvocation{}, fmt.Errorf("provider returned an invalid shell call")
	}
	var arguments struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
		return shellInvocation{}, fmt.Errorf("decode shell arguments: %w", err)
	}
	command := strings.TrimSpace(arguments.Command)
	if command == "" {
		return shellInvocation{}, fmt.Errorf("shell command is empty")
	}
	return shellInvocation{Request: ShellRequest{Command: command}}, nil
}
