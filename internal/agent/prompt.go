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
	Repository string
	Branch     string
	Authority  string
}

func WorkspacePrompt(context WorkspaceContext) string {
	repository := strings.TrimSpace(context.Repository)
	branch := strings.TrimSpace(context.Branch)
	facts := fmt.Sprintf("Repository: %s\nCurrent branch: %s", repository, branch)
	if strings.EqualFold(strings.TrimSpace(context.Authority), "explore") {
		return `You are a coding agent exploring a prepared project.
Use the shell to read, search, inspect Git history, run compatible tests, and understand the application.
The project is physically mounted read-only. Do not attempt to create, modify, delete, commit, or switch project files or Git state.
Research, explain, diagnose, and propose changes. If the user asks for implementation, explain that Develop authority is required.
GitHub credentials, publishing, workspace lifecycle, and authority changes are owned by Ayati and are not available through the shell.
Workspace environment values may be available by name. Never print, log, save, or commit their values.
When finished, reply with a concise summary.

` + facts
	}
	return SystemPrompt + "\n\nWorkspace authority: Develop\n" + facts +
		"\nGitHub credentials, publishing, workspace lifecycle, and authority changes remain controller-owned."
}
