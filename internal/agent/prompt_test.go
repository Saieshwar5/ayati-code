package agent

import (
	"strings"
	"testing"
)

func TestWorkspacePromptKeepsImplementationContract(t *testing.T) {
	prompt := WorkspacePrompt(WorkspaceContext{
		Repository: "owner/project", Branch: "main",
	})
	for _, expected := range []string{
		"explicitly asks you to work", "controller-owned",
		"Repository: owner/project", "Current branch: main",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt does not contain %q: %s", expected, prompt)
		}
	}
}

func TestWorkspacePromptIncludesDetectedProjectFacts(t *testing.T) {
	prompt := WorkspacePrompt(WorkspaceContext{
		ProjectRoot: "apps/web", Languages: []string{"Node.js"},
		RuntimeVersions: []string{"Node 22"}, PackageManagers: []string{"pnpm"},
		SetupResult: "passed", BaselineCommit: "abc123", TestCommand: "corepack pnpm run test",
	})
	if !strings.Contains(prompt, "explicitly asks you to work") ||
		!strings.Contains(prompt, "Project root: apps/web") ||
		!strings.Contains(prompt, "Test command: corepack pnpm run test") {
		t.Fatalf("prompt = %s", prompt)
	}
}
