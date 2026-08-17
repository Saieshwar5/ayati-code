package workspace

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

func TestAgentCatalogProtectsBuiltInAndMaintainsOneDefault(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	values, err := store.ListAgents(context.Background(), false)
	if err != nil || len(values) != 1 || values[0].ID != agent.BuiltinAgentID || !values[0].Default {
		t.Fatalf("initial agents = %#v, error = %v", values, err)
	}
	if _, err := store.UpdateAgent(context.Background(), agent.BuiltinAgentID, agent.DefinitionInput{
		Name: "Changed", MaxSteps: 10,
	}); err == nil || !strings.Contains(err.Error(), "cannot be edited") {
		t.Fatalf("built-in update error = %v", err)
	}
	if err := store.ArchiveAgent(context.Background(), agent.BuiltinAgentID); err == nil {
		t.Fatal("ArchiveAgent accepted built-in agent")
	}
	created, err := store.CreateAgent(context.Background(), agent.DefinitionInput{
		Name: "Test specialist", Emoji: "🧪", Description: "Improves tests",
		ProviderID: agent.FireworksProviderID, Model: "test-model", MaxSteps: 8,
		ShellEnabled: true, Instructions: "Inspect failures first.",
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	updated, err := store.UpdateAgent(context.Background(), created.ID, agent.DefinitionInput{
		Name: "Test specialist", Emoji: "🧪", Description: "Improves tests",
		ProviderID: agent.FireworksProviderID, Model: "test-model", MaxSteps: 9,
		ShellEnabled: true, Instructions: "Inspect failures first.",
	})
	if err != nil || updated.Revision != 2 || updated.MaxSteps != 9 {
		t.Fatalf("updated agent = %#v, error = %v", updated, err)
	}
	selected, err := store.SetDefaultAgent(context.Background(), created.ID)
	if err != nil || !selected.Default {
		t.Fatalf("default agent = %#v, error = %v", selected, err)
	}
	if err := store.ArchiveAgent(context.Background(), created.ID); err == nil ||
		!strings.Contains(err.Error(), "choose another default") {
		t.Fatalf("default archive error = %v", err)
	}
}

func TestAgentCatalogDefaultsNewSessionsAndReassignsArchivedAgent(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	custom, err := store.CreateAgent(context.Background(), agent.DefinitionInput{
		Name: "Reviewer", ProviderID: agent.FireworksProviderID, MaxSteps: 6,
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if _, err := store.SetDefaultAgent(context.Background(), custom.ID); err != nil {
		t.Fatalf("SetDefaultAgent: %v", err)
	}
	session, err := store.CreateSession(context.Background(), value.ID, "Review")
	if err != nil || session.SelectedAgentID != custom.ID {
		t.Fatalf("new session = %#v, error = %v", session, err)
	}
	if _, err := store.SetDefaultAgent(context.Background(), agent.BuiltinAgentID); err != nil {
		t.Fatalf("restore built-in default: %v", err)
	}
	if err := store.ArchiveAgent(context.Background(), custom.ID); err != nil {
		t.Fatalf("ArchiveAgent: %v", err)
	}
	loaded, err := store.GetSession(context.Background(), value.ID, session.ID)
	if err != nil || loaded.SelectedAgentID != agent.BuiltinAgentID {
		t.Fatalf("reassigned session = %#v, error = %v", loaded, err)
	}
}

func TestAgentDefinitionValidationRejectsUnsupportedConfiguration(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, input := range []agent.DefinitionInput{
		{},
		{Name: "Too many steps", MaxSteps: 21},
		{Name: "Unknown provider", ProviderID: "unknown", MaxSteps: 5},
	} {
		if _, err := store.CreateAgent(context.Background(), input); err == nil {
			t.Fatalf("CreateAgent accepted %#v", input)
		}
	}
}
