package agent

import (
	"fmt"
	"strings"
)

const SystemPrompt = `You are a coding agent working in the current directory.
Use the shell to inspect, edit, and test the project.
The workspace is persistent for this conversation. You may create, modify, and delete files there when the user explicitly asks you to work on the project.
The workspace may provide environment variables for development. Use them by name when needed, but never print, log, save, or commit their values.
When the user asks for explanation, review, or a plan, do not modify files unless they subsequently authorize implementation.
Make focused changes and continue until the task is complete.
When finished, reply with a concise summary.`

type WorkspaceContext struct {
	Repository       string
	Branch           string
	ProjectRoot      string
	Languages        []string
	RuntimeVersions  []string
	PackageManagers  []string
	SetupResult      string
	BaselineCommit   string
	TestCommand      string
	LintCommand      string
	TypecheckCommand string
	BuildCommand     string
}

func WorkspacePrompt(context WorkspaceContext) string {
	repository := strings.TrimSpace(context.Repository)
	branch := strings.TrimSpace(context.Branch)
	facts := workspaceFacts(context, repository, branch)
	return SystemPrompt + "\n\n" + facts +
		"\nGitHub credentials, publishing, and workspace lifecycle remain controller-owned."
}

func workspaceFacts(context WorkspaceContext, repository, branch string) string {
	var facts strings.Builder
	fmt.Fprintf(&facts, "Repository: %s\nCurrent branch: %s", repository, branch)
	if strings.TrimSpace(context.ProjectRoot) == "" {
		return facts.String()
	}
	fmt.Fprintf(&facts, "\nProject root: %s", context.ProjectRoot)
	for _, fact := range []struct {
		label  string
		values []string
	}{
		{"Languages", context.Languages}, {"Runtimes", context.RuntimeVersions},
		{"Package managers", context.PackageManagers},
	} {
		if len(fact.values) > 0 {
			fmt.Fprintf(&facts, "\n%s: %s", fact.label, strings.Join(fact.values, ", "))
		}
	}
	if context.SetupResult != "" {
		fmt.Fprintf(&facts, "\nPreparation: setup %s", context.SetupResult)
	}
	if context.BaselineCommit != "" {
		fmt.Fprintf(&facts, "\nBaseline commit: %s", context.BaselineCommit)
	}
	for _, fact := range []struct{ label, command string }{
		{"Test command", context.TestCommand}, {"Lint command", context.LintCommand},
		{"Typecheck command", context.TypecheckCommand}, {"Build command", context.BuildCommand},
	} {
		if fact.command != "" {
			fmt.Fprintf(&facts, "\n%s: %s", fact.label, fact.command)
		}
	}
	return facts.String()
}
