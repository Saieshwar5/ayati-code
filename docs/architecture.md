# Ayati Micro Architecture

## Boundary

Ayati is one local Go process with one Fireworks client and one model-facing tool named `shell`. It is a trusted-local coding harness, not a sandbox or hosted execution platform.

The runtime flow is intentionally linear:

```text
user message
  -> append to JSONL session
  -> request one Fireworks decision
  -> final text: append and stop
  -> shell call: append, execute, append result
  -> continue, up to 20 decisions
```

The twentieth shell result is recorded before the loop stops. The harness does not make an extra provider call after the limit.

## Components

- `internal/app` owns flags, startup, commands, and the active session.
- `internal/agent` owns the prompt, message types, and 20-step loop.
- `internal/fireworks` owns the single non-streaming Chat Completions request.
- `internal/shell` runs `/bin/sh -lc` in the workspace with fixed reliability bounds.
- `internal/session` stores one append-only JSONL file per session.
- `internal/ui` renders the plain line-oriented terminal interface.

The module uses only the Go standard library.

## Sessions

The first JSONL record contains session ID, canonical workspace, model, and creation time. Remaining records are exact user, assistant, and tool messages. Resume loads and replays the complete file. There is no database, migration, snapshot, compaction, recovery state machine, or context accounting.

## Shell and trust

Commands run with the current user's host permissions. Ayati removes `FIREWORKS_API_KEY` from the child environment and enforces command size, output size, timeout, process-group cancellation, and workspace working directory. These are reliability controls, not a security boundary; a same-user process may still access host files and processes.

## Deferred work

Additional providers, model discovery, credential storage, TUI, attachments, network policy, sandboxing, context compaction, and richer session lifecycle remain outside this micro-harness.
