package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var ErrStepLimit = errors.New("agent reached the 20-step limit")

type shellInvocation struct {
	Command string
	Purpose string
}

type Loop struct {
	Provider Provider
	Shell    Shell
	Recorder Recorder
	Observer Observer
	Model    string
}

func (l Loop) Run(ctx context.Context, history *[]Message, userText string) (Completion, error) {
	if l.Provider == nil || l.Shell == nil || l.Recorder == nil || history == nil {
		return Completion{}, fmt.Errorf("agent loop is not fully configured")
	}
	if strings.TrimSpace(l.Model) == "" || strings.TrimSpace(userText) == "" {
		return Completion{}, fmt.Errorf("model and user request are required")
	}
	if err := ctx.Err(); err != nil {
		return Completion{}, err
	}
	user := Message{Role: "user", Content: userText}
	if err := l.record(history, user); err != nil {
		return Completion{}, err
	}
	for step := 1; step <= MaxSteps; step++ {
		if err := ctx.Err(); err != nil {
			return Completion{Steps: step - 1}, err
		}
		if l.Observer != nil {
			l.Observer.Step(step, MaxSteps)
		}
		message, err := l.Provider.Next(ctx, Request{
			Model: l.Model, SystemPrompt: SystemPrompt, Messages: append([]Message(nil), (*history)...),
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
			return Completion{Steps: step}, fmt.Errorf("provider returned %d tool calls; Ayati permits one", len(message.ToolCalls))
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
		invocation, err := parseShellCall(call)
		if err != nil {
			return Completion{Steps: step}, err
		}
		if err := l.record(history, message); err != nil {
			return Completion{Steps: step}, err
		}
		if l.Observer != nil {
			l.Observer.ToolCall(invocation.Purpose, invocation.Command)
		}
		result := l.Shell.Run(ctx, invocation.Command)
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
	return Completion{Steps: MaxSteps, Exhausted: true}, ErrStepLimit
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
		Purpose string `json:"purpose"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
		return shellInvocation{}, fmt.Errorf("decode shell arguments: %w", err)
	}
	if strings.TrimSpace(arguments.Command) == "" {
		return shellInvocation{}, fmt.Errorf("shell command is empty")
	}
	purpose := strings.Join(strings.Fields(arguments.Purpose), " ")
	if purpose == "" {
		return shellInvocation{}, fmt.Errorf("shell purpose is empty")
	}
	if utf8.RuneCountInString(purpose) > MaxShellPurposeLength {
		return shellInvocation{}, fmt.Errorf("shell purpose exceeds %d characters", MaxShellPurposeLength)
	}
	return shellInvocation{Command: arguments.Command, Purpose: purpose}, nil
}
