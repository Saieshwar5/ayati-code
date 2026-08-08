package runtime

import (
	"context"
	"errors"
)

type scriptedModel struct {
	conversation Conversation
}

func (m scriptedModel) Start(systemPrompt, userPrompt string, tool ToolDefinition) (Conversation, error) {
	if systemPrompt == "" || userPrompt == "" || tool.Name != ShellToolName {
		return nil, errors.New("invalid model start")
	}
	return m.conversation, nil
}

type scriptedConversation struct {
	decisions               []Decision
	errors                  []error
	observations            []*ToolResult
	calls                   int
	maintenanceDecisions    []Decision
	maintenanceErrors       []error
	maintenanceObservations []*ToolResult
	maintenanceInstructions []string
	maintenanceCalls        int
}

func (c *scriptedConversation) Next(_ context.Context, observation *ToolResult) (Decision, error) {
	if observation != nil {
		copy := *observation
		c.observations = append(c.observations, &copy)
	} else {
		c.observations = append(c.observations, nil)
	}
	index := c.calls
	c.calls++
	if index < len(c.errors) && c.errors[index] != nil {
		return Decision{}, c.errors[index]
	}
	if index >= len(c.decisions) {
		return Decision{}, errors.New("unexpected model call")
	}
	return c.decisions[index], nil
}

func (c *scriptedConversation) RespondWithoutTools(_ context.Context, observation *ToolResult, instruction string) (Decision, error) {
	if observation != nil {
		copy := *observation
		c.maintenanceObservations = append(c.maintenanceObservations, &copy)
	} else {
		c.maintenanceObservations = append(c.maintenanceObservations, nil)
	}
	c.maintenanceInstructions = append(c.maintenanceInstructions, instruction)
	index := c.maintenanceCalls
	c.maintenanceCalls++
	if index < len(c.maintenanceErrors) && c.maintenanceErrors[index] != nil {
		return Decision{}, c.maintenanceErrors[index]
	}
	if index >= len(c.maintenanceDecisions) {
		return Decision{}, errors.New("unexpected tool-disabled model call")
	}
	return c.maintenanceDecisions[index], nil
}

type queuedModel struct {
	conversations []Conversation
	userPrompts   []string
}

func (m *queuedModel) Start(systemPrompt, userPrompt string, tool ToolDefinition) (Conversation, error) {
	if systemPrompt == "" || userPrompt == "" || tool.Name != ShellToolName {
		return nil, errors.New("invalid model start")
	}
	m.userPrompts = append(m.userPrompts, userPrompt)
	if len(m.conversations) == 0 {
		return nil, errors.New("unexpected model start")
	}
	conversation := m.conversations[0]
	m.conversations = m.conversations[1:]
	return conversation, nil
}

type fakeShell struct {
	commands []string
	results  []ToolResult
}

func (s *fakeShell) Run(_ context.Context, command string) (ToolResult, error) {
	s.commands = append(s.commands, command)
	if len(s.results) == 0 {
		return ToolResult{}, nil
	}
	result := s.results[0]
	s.results = s.results[1:]
	return result, nil
}

type eventRecorder struct {
	events []Event
}

func (s *eventRecorder) Emit(event Event) error {
	s.events = append(s.events, event)
	return nil
}

type blockingConversation struct{}

func (blockingConversation) Next(ctx context.Context, _ *ToolResult) (Decision, error) {
	<-ctx.Done()
	return Decision{}, ctx.Err()
}

func (blockingConversation) RespondWithoutTools(ctx context.Context, _ *ToolResult, _ string) (Decision, error) {
	<-ctx.Done()
	return Decision{}, ctx.Err()
}

type failingSink struct {
	calls  int
	failAt int
}

func (s *failingSink) Emit(Event) error {
	s.calls++
	if s.calls == s.failAt {
		return errors.New("journal unavailable")
	}
	return nil
}
