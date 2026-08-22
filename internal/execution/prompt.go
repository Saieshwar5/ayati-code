package execution

import "strconv"

const systemPrompt = `You are Perpetual's coding agent. You have exactly one tool: shell(command).
Run commands to inspect the repository, install dependencies, and make changes.
Git commits, pushes, and pull requests are performed by the controller and are never your responsibility.`

// summarizationPrompt asks the model to produce a structured checkpoint summary
// of history being compacted. It mirrors pi's structured compaction format.
const summarizationPrompt = `The conversation above is history for a coding agent session. Create a structured context checkpoint summary that another LLM can use to continue the work.

Use this exact format:

## Goal
[What is the user trying to accomplish?]

## Constraints & Preferences
- [Any constraints or preferences; or "(none)"]

## Progress
### Done
- [x] [Completed work]

### In Progress
- [ ] [Current work]

### Blocked
- [Open blockers; or "(none)"]

## Key Decisions
- **[Decision]**: [Rationale]

## Next Steps
1. [Ordered next steps]

## Critical Context
- [Exact file paths, function names, command results, or error messages needed to continue]
- [Or "(none)"]

Preserve exact file paths, function names, and error messages. Keep it concise.`

// updateSummarizationPrompt merges new history into an existing summary.
const updateSummarizationPrompt = `The new conversation above must be incorporated into the existing summary below.

<previous-summary>
%s
</previous-summary>

Rules:
- Preserve all existing information.
- Add new progress, decisions, and context.
- Move items from "In Progress" to "Done" when completed.
- Preserve exact file paths, function names, and error messages.

Use the exact same structured format:

## Goal
## Constraints & Preferences
## Progress (Done / In Progress / Blocked)
## Key Decisions
## Next Steps
## Critical Context`

func formatExit(exitCode float64) string {
	return strconv.FormatFloat(exitCode, 'f', 0, 64)
}
