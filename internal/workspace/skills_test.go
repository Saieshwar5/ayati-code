package workspace

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/agent"
)

func TestSkillCatalogAttachesOrderedGuidanceToCustomAgent(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	first, err := store.CreateSkill(context.Background(), agent.SkillInput{
		Name: "Testing", Description: "Test guidance", Markdown: "Run focused tests first.",
	})
	if err != nil {
		t.Fatalf("CreateSkill first: %v", err)
	}
	second, err := store.CreateSkill(context.Background(), agent.SkillInput{
		Name: "Go review", Markdown: "Check context cancellation.",
	})
	if err != nil {
		t.Fatalf("CreateSkill second: %v", err)
	}
	definition, err := store.CreateAgent(context.Background(), agent.DefinitionInput{
		Name: "Reviewer", ProviderID: agent.FireworksProviderID, MaxSteps: 8,
		SkillIDs: []string{second.ID, first.ID},
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if len(definition.SkillIDs) != 2 || definition.SkillIDs[0] != second.ID || definition.SkillIDs[1] != first.ID {
		t.Fatalf("skill IDs = %#v", definition.SkillIDs)
	}
	attached, err := store.AgentSkills(context.Background(), definition.ID)
	if err != nil || len(attached) != 2 || attached[0].ID != second.ID || attached[1].ID != first.ID {
		t.Fatalf("attached skills = %#v, error = %v", attached, err)
	}
	loaded, err := store.GetSkill(context.Background(), first.ID)
	if err != nil || loaded.AttachedAgents != 1 {
		t.Fatalf("loaded skill = %#v, error = %v", loaded, err)
	}
	duplicate, err := store.DuplicateAgent(context.Background(), definition.ID)
	if err != nil || len(duplicate.SkillIDs) != 2 || duplicate.SkillIDs[0] != second.ID {
		t.Fatalf("duplicate = %#v, error = %v", duplicate, err)
	}
}

func TestSkillCatalogProtectsAttachedAndArchivedSkills(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	skill, err := store.CreateSkill(context.Background(), agent.SkillInput{
		Name: "Safety", Markdown: "Inspect boundaries.",
	})
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}
	definition, err := store.CreateAgent(context.Background(), agent.DefinitionInput{
		Name: "Reviewer", MaxSteps: 5, SkillIDs: []string{skill.ID},
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if err := store.ArchiveSkill(context.Background(), skill.ID); err == nil ||
		!strings.Contains(err.Error(), "detach") {
		t.Fatalf("archive attached skill error = %v", err)
	}
	definition.SkillIDs = nil
	if _, err := store.UpdateAgent(context.Background(), definition.ID, agent.DefinitionInput{
		Name: definition.Name, ProviderID: definition.ProviderID, MaxSteps: definition.MaxSteps,
	}); err != nil {
		t.Fatalf("detach skill: %v", err)
	}
	if err := store.ArchiveSkill(context.Background(), skill.ID); err != nil {
		t.Fatalf("ArchiveSkill: %v", err)
	}
	if _, err := store.CreateAgent(context.Background(), agent.DefinitionInput{
		Name: "Invalid", MaxSteps: 5, SkillIDs: []string{skill.ID},
	}); err == nil || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("attach archived skill error = %v", err)
	}
	restored, err := store.RestoreSkill(context.Background(), skill.ID)
	if err != nil || restored.ArchivedAt != nil {
		t.Fatalf("restored skill = %#v, error = %v", restored, err)
	}
}

func TestSkillValidationRejectsEmptyAndOversizedConfiguration(t *testing.T) {
	if _, err := agent.NormalizeSkill(agent.SkillInput{}); err == nil {
		t.Fatal("NormalizeSkill accepted empty input")
	}
	if _, err := agent.NormalizeSkill(agent.SkillInput{
		Name: "Huge", Markdown: strings.Repeat("x", (32<<10)+1),
	}); err == nil {
		t.Fatal("NormalizeSkill accepted oversized Markdown")
	}
}

func TestSkillCatalogEnforcesCombinedMarkdownLimitOnUpdates(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	var skillIDs []string
	for _, name := range []string{"First", "Second", "Third"} {
		value, createErr := store.CreateSkill(context.Background(), agent.SkillInput{
			Name: name, Markdown: strings.Repeat(strings.ToLower(name[:1]), 20<<10),
		})
		if createErr != nil {
			t.Fatalf("CreateSkill %s: %v", name, createErr)
		}
		skillIDs = append(skillIDs, value.ID)
	}
	if _, err := store.CreateAgent(context.Background(), agent.DefinitionInput{
		Name: "Bounded", MaxSteps: 5, SkillIDs: skillIDs,
	}); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if _, err := store.UpdateSkill(context.Background(), skillIDs[0], agent.SkillInput{
		Name: "First", Markdown: strings.Repeat("x", 30<<10),
	}); err == nil || !strings.Contains(err.Error(), "exceed") {
		t.Fatalf("oversized combined update error = %v", err)
	}
}
