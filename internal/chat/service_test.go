package chat

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/agent"
	"github.com/Saieshwar5/ayati-code/internal/workspace"
)

type fakeRuntime struct {
	shell     agent.Shell
	workspace workspace.Workspace
}

func (f fakeRuntime) Shell(_ context.Context, _ string) (agent.Shell, workspace.Workspace, error) {
	return f.shell, f.workspace, nil
}

type scriptedProvider struct {
	messages []agent.Message
	requests []agent.Request
}

func (p *scriptedProvider) Next(_ context.Context, request agent.Request) (agent.Message, error) {
	p.requests = append(p.requests, request)
	message := p.messages[0]
	p.messages = p.messages[1:]
	return message, nil
}

type blockingProvider struct{ started chan struct{} }

func (p blockingProvider) Next(ctx context.Context, _ agent.Request) (agent.Message, error) {
	close(p.started)
	<-ctx.Done()
	return agent.Message{}, ctx.Err()
}

type fakeShell struct{ commands []string }

func (s *fakeShell) Execute(_ context.Context, request agent.ShellRequest) agent.ShellResult {
	s.commands = append(s.commands, request.Command)
	return agent.ShellResult{Command: request.Command, ExitCode: 0, Stdout: "ok"}
}

func TestServiceKeepsConversationAndSandboxAcrossTurns(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), workspace.Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.UpdateStatus(context.Background(), value.ID, workspace.StatusReady, ""); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	value.Profile = &workspace.ProjectProfile{
		ProjectRoot: "apps/web", Languages: []string{"Node.js"},
		RuntimeVersions: []string{"Node 22"}, PackageManagers: []string{"pnpm"},
		SetupResult: "passed", BaselineCommit: "abc123", TestCommand: "corepack pnpm run test",
	}
	sessions, err := store.ListSessions(context.Background(), value.ID)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %#v, error = %v", sessions, err)
	}
	provider := &scriptedProvider{messages: []agent.Message{
		{Role: "assistant", ToolCalls: []agent.ToolCall{{
			ID: "call-1", Type: "function", Function: agent.FunctionCall{Name: "shell", Arguments: `{"command":"pwd"}`},
		}}},
		{Role: "assistant", Content: "done"},
	}}
	shell := &fakeShell{}
	service, err := New(store, fakeRuntime{shell: shell, workspace: value}, provider, "test-model")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	completion, err := service.Send(context.Background(), value.ID, sessions[0].ID, "work on it")
	if err != nil || completion.Text != "done" || len(shell.commands) != 1 {
		t.Fatalf("completion = %#v, commands = %#v, error = %v", completion, shell.commands, err)
	}
	if len(provider.requests) == 0 ||
		!strings.Contains(provider.requests[0].SystemPrompt, "physically mounted read-only") ||
		!strings.Contains(provider.requests[0].SystemPrompt, "Project root: apps/web") {
		t.Fatalf("system prompt = %q", provider.requests[0].SystemPrompt)
	}
	messages, err := service.Messages(context.Background(), value.ID, sessions[0].ID)
	if err != nil || len(messages) != 4 || messages[0].Role != "user" || messages[3].Content != "done" {
		t.Fatalf("messages = %#v, error = %v", messages, err)
	}
	loaded, _ := store.GetSession(context.Background(), value.ID, sessions[0].ID)
	if loaded.Status != workspace.SessionStatusReview {
		t.Fatalf("status = %s", loaded.Status)
	}
}

func TestServiceCancelsActiveWorkspaceRun(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), workspace.Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "ayati/change", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.UpdateStatus(context.Background(), value.ID, workspace.StatusReady, ""); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	sessions, err := store.ListSessions(context.Background(), value.ID)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %#v, error = %v", sessions, err)
	}
	started := make(chan struct{})
	service, err := New(store, fakeRuntime{shell: &fakeShell{}}, blockingProvider{started: started}, "test-model")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	finished := make(chan error, 1)
	go func() {
		_, runErr := service.Send(context.Background(), value.ID, sessions[0].ID, "work on it")
		finished <- runErr
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("agent run did not start")
	}
	service.CancelAndWait(value.ID)
	select {
	case runErr := <-finished:
		if !errors.Is(runErr, context.Canceled) {
			t.Fatalf("Send error = %v", runErr)
		}
	case <-time.After(time.Second):
		t.Fatal("agent run did not stop")
	}
}

func TestServiceRejectsConcurrentWorkspaceRun(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), workspace.Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "ayati/change", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.UpdateStatus(context.Background(), value.ID, workspace.StatusReady, ""); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	sessions, err := store.ListSessions(context.Background(), value.ID)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	second, err := store.CreateSession(context.Background(), value.ID, "Second session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	started := make(chan struct{})
	service, err := New(store, fakeRuntime{shell: &fakeShell{}}, blockingProvider{started: started}, "test-model")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	finished := make(chan error, 1)
	go func() {
		_, runErr := service.Send(context.Background(), value.ID, sessions[0].ID, "first run")
		finished <- runErr
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first run did not start")
	}
	_, err = service.Send(context.Background(), value.ID, second.ID, "second run")
	if err == nil || err.Error() != "another session is already running in this workspace" {
		t.Fatalf("concurrent Send error = %v", err)
	}
	service.Cancel(value.ID)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("first run did not stop")
	}
}
