package agent

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/sai-eshwar/no-nonsense-coding-ai/internal/chat"
	"github.com/sai-eshwar/no-nonsense-coding-ai/internal/session"
	"github.com/sai-eshwar/no-nonsense-coding-ai/internal/shell"
)

type scriptedProvider struct {
	responses []chat.Message
	calls     int
}

func (p *scriptedProvider) Complete(_ context.Context, _ []chat.Message) (chat.Message, error) {
	response := p.responses[p.calls]
	p.calls++
	return response, nil
}

type fakeShell struct {
	commands []string
}

type compactingProvider struct {
	contexts     [][]chat.Message
	summaryCalls int
	responses    []chat.Message
	contextLimit int
}

func (p *compactingProvider) Complete(_ context.Context, messages []chat.Message) (chat.Message, error) {
	copy := append([]chat.Message(nil), messages...)
	p.contexts = append(p.contexts, copy)
	if len(p.responses) == 0 {
		return chat.Message{Content: "Done."}, nil
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}

func (p *compactingProvider) Summarize(_ context.Context, _ string) (string, error) {
	p.summaryCalls++
	return "Task: continue the coding work. Completed: earlier shell steps. Next: finish and verify.", nil
}

func (p *compactingProvider) ContextLimit(_ context.Context) (int, error) {
	return p.contextLimit, nil
}

func (s *fakeShell) Run(_ context.Context, command string) (shell.Result, error) {
	s.commands = append(s.commands, command)
	return shell.Result{Command: command, Stdout: "file.go\n", ExitCode: 0}, nil
}

func TestPromptRunsShellAndPersistsConversation(t *testing.T) {
	store := session.Store{Dir: t.TempDir()}
	current, err := store.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	provider := &scriptedProvider{responses: []chat.Message{
		{ToolCalls: []chat.ToolCall{{ID: "call-1", Type: "function", Function: chat.FunctionCall{Name: "shell", Arguments: `{"command":"ls"}`}}}},
		{Content: "Done."},
	}}
	executor := &fakeShell{}
	var output bytes.Buffer
	a := Agent{Provider: provider, Shell: executor, Store: store, Session: current, Output: &output}
	if err := a.Prompt(context.Background(), "inspect the project"); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if len(executor.commands) != 1 || executor.commands[0] != "ls" {
		t.Fatalf("unexpected shell calls: %v", executor.commands)
	}
	if len(current.Messages) != 4 {
		t.Fatalf("got %d persisted messages, want 4", len(current.Messages))
	}
	if output.String() == "" {
		t.Fatal("expected terminal output")
	}
}

func TestContextCompactionAlwaysPreservesCurrentUser(t *testing.T) {
	store := session.Store{Dir: t.TempDir()}
	current, err := store.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 8; index++ {
		messages := []chat.Message{
			{Role: "user", Content: "older request with substantial details " + strings.Repeat("x", 300)},
			{Role: "assistant", ToolCalls: []chat.ToolCall{{ID: fmt.Sprintf("call-%d", index), Type: "function", Function: chat.FunctionCall{Name: "shell", Arguments: `{"command":"inspect files"}`}}}},
			{Role: "tool", ToolCallID: fmt.Sprintf("call-%d", index), Content: strings.Repeat("result ", 120)},
			{Role: "assistant", Content: "older task completed"},
		}
		for _, message := range messages {
			if err := store.Append(current, message); err != nil {
				t.Fatal(err)
			}
		}
	}
	provider := &compactingProvider{contextLimit: 1200}
	a := Agent{
		Provider:              provider,
		Shell:                 &fakeShell{},
		Store:                 store,
		Session:               current,
		Output:                &bytes.Buffer{},
		MaxContextToolPairs:   3,
		ContextPercent:        70,
		FallbackContextTokens: 1200,
	}
	const currentRequest = "finish the current website and verify every page"
	if err := a.Prompt(context.Background(), currentRequest); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if provider.summaryCalls == 0 || current.Summary == nil {
		t.Fatal("expected durable context compaction")
	}
	if len(provider.contexts) != 1 || !hasUserContent(provider.contexts[0], currentRequest) {
		t.Fatalf("current user request was lost: %#v", provider.contexts)
	}
	if countToolResults(provider.contexts[0]) > 3 {
		t.Fatalf("too many exact tool results retained: %d", countToolResults(provider.contexts[0]))
	}
}

func TestEmptyProviderResponseIsRetriedNotPersisted(t *testing.T) {
	store := session.Store{Dir: t.TempDir()}
	current, err := store.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	provider := &compactingProvider{
		contextLimit: 128000,
		responses: []chat.Message{
			{},
			{Content: "Recovered response."},
		},
	}
	a := Agent{Provider: provider, Shell: &fakeShell{}, Store: store, Session: current, Output: &bytes.Buffer{}}
	if err := a.Prompt(context.Background(), "hello"); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if len(provider.contexts) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(provider.contexts))
	}
	if len(current.Messages) != 2 || current.Messages[1].Content != "Recovered response." {
		t.Fatalf("empty response was persisted: %#v", current.Messages)
	}
}

func hasUserContent(messages []chat.Message, content string) bool {
	for _, message := range messages {
		if message.Role == "user" && message.Content == content {
			return true
		}
	}
	return false
}
