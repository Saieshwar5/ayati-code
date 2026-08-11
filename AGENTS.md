# Repository Guidelines

## Project Scope

Ayati is a minimal, standard-library-only Go coding harness for Linux. It has one provider (Fireworks), one model-facing tool (`shell`), one sequential agent loop limited to 20 decisions, and filesystem-backed JSONL sessions.

Keep this boundary small. Do not add providers, dependencies, modes, planners, databases, snapshots, background services, or compatibility layers without explicit approval.

## Package Ownership

- `cmd/ayati/`: process entry point and signal setup only.
- `internal/app/`: flags, startup, terminal commands, and component wiring.
- `internal/agent/`: system prompt, shared message contracts, and agent loop.
- `internal/config/`: private saved API key and default model configuration.
- `internal/fireworks/`: Fireworks HTTP request and response handling.
- `internal/shell/`: bounded `/bin/sh -lc` execution in the workspace.
- `internal/session/`: append-only JSONL session creation, loading, and listing.
- `internal/ui/`: plain line-oriented terminal input and rendering.
- `docs/`: architecture and important design decisions.

Keep logic in the package that owns the responsibility. `internal/app` may connect packages, but infrastructure packages should not depend on it.

## Development Commands

```bash
go run -buildvcs=false ./cmd/ayati
go test -buildvcs=false ./...
go test -buildvcs=false -race ./...
go vet -buildvcs=false ./...
CGO_ENABLED=0 go build -buildvcs=false -trimpath -o ayati ./cmd/ayati
gofmt -w cmd internal
```

Before handoff, run formatting, tests, race checks, vet, and the CGO-disabled build.

## Code and Tests

Use idiomatic Go and `gofmt`. Prefer concrete types, small consumer-owned interfaces, contextual errors, and short responsibility-focused files. Keep source files below 300 lines. Exported names use PascalCase; unexported names use camelCase.

Colocate tests as `*_test.go` and name them `TestFeatureBehavior`. Cover changed behavior and failure paths. Fireworks tests must use local HTTP test servers and must never require a real API key or internet access.

## Security and Runtime Rules

Shell commands run directly with the current user's host permissions; this is not a sandbox. Preserve command, output, timeout, workspace, and process-group cancellation bounds. Ctrl+C must stop active model and shell work, while `/quit` exits from the prompt.

Each shell call requires a short purpose for display. Treat it only as untrusted explanatory metadata; never use it as an execution or security boundary.

Persist the Fireworks key only in the private configuration file. Never place it in sessions, logs, terminal output, tests, or shell child environments.

## Git Changes

Keep commits focused and use short imperative messages such as `feat: add session resume`. Do not mix unrelated cleanup with behavior changes.
