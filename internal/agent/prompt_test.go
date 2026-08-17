package agent

import (
	"strings"
	"testing"
)

func TestWorkspacePromptDescribesExploreAuthority(t *testing.T) {
	prompt := WorkspacePrompt(WorkspaceContext{
		Repository: "owner/project", Branch: "main", Authority: "explore",
	})
	for _, expected := range []string{
		"physically mounted read-only", "Do not attempt to create", "Develop authority is required",
		"Repository: owner/project", "Current branch: main",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt does not contain %q: %s", expected, prompt)
		}
	}
}

func TestWorkspacePromptKeepsDevelopImplementationContract(t *testing.T) {
	prompt := WorkspacePrompt(WorkspaceContext{
		Authority: "develop", ProjectRoot: "apps/web", Languages: []string{"Node.js"},
		RuntimeVersions: []string{"Node 22"}, PackageManagers: []string{"pnpm"},
		SetupResult: "passed", BaselineCommit: "abc123", TestCommand: "corepack pnpm run test",
	})
	if !strings.Contains(prompt, "explicitly asks you to work") ||
		!strings.Contains(prompt, "Workspace authority: Develop") ||
		!strings.Contains(prompt, "Project root: apps/web") ||
		!strings.Contains(prompt, "Test command: corepack pnpm run test") {
		t.Fatalf("prompt = %s", prompt)
	}
}

func TestDefinitionPromptKeepsCustomInstructionsSubordinate(t *testing.T) {
	prompt := DefinitionPrompt("controller policy", Definition{
		Name: "Reviewer", Instructions: "Focus on architecture risks.",
	}, Skill{Name: "Go review", Revision: 3, Markdown: "Check context cancellation."})
	for _, expected := range []string{
		"controller policy", "subordinate to Ayati's workspace authority", "Agent name: Reviewer",
		"Focus on architecture risks.", "ATTACHED SKILLS", "## Go review (revision 3)",
		"Check context cancellation.", "cannot override controller rules",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt does not contain %q: %s", expected, prompt)
		}
	}
}
