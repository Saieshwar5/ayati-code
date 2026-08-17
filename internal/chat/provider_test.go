package chat

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Saieshwar5/ayati-code/internal/agent"
	modelprovider "github.com/Saieshwar5/ayati-code/internal/provider"
	"github.com/Saieshwar5/ayati-code/internal/workspace"
)

func TestServiceResolvesTheSelectedAgentProvider(t *testing.T) {
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
	definition, err := store.CreateAgent(context.Background(), agent.DefinitionInput{
		Name: "Other provider", ProviderID: "other", MaxSteps: 4,
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	sessions, err := store.ListSessions(context.Background(), value.ID)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %#v, error = %v", sessions, err)
	}
	if _, err := store.SelectSessionAgent(context.Background(), value.ID, sessions[0].ID, definition.ID); err != nil {
		t.Fatalf("SelectSessionAgent: %v", err)
	}
	selected := &scriptedProvider{messages: []agent.Message{{Role: "assistant", Content: "other done"}}}
	registry, err := modelprovider.New(modelprovider.Registration{
		Definition: modelprovider.Definition{ID: "other", Name: "Other", Protocol: "test"},
		Client:     selected, DefaultModel: "other-model",
	})
	if err != nil {
		t.Fatalf("provider.New: %v", err)
	}
	service, err := New(store, fakeRuntime{shell: &fakeShell{}, workspace: value}, registry)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	completion, err := service.Send(context.Background(), value.ID, sessions[0].ID, "use selected provider")
	if err != nil || completion.Text != "other done" || len(selected.requests) != 1 ||
		selected.requests[0].Model != "other-model" {
		t.Fatalf("completion = %#v, requests = %#v, error = %v", completion, selected.requests, err)
	}
}
