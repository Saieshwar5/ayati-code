# Repository Guidelines

## Project Structure & Module Organization

Ayati is a standard-library-only Go coding harness. `cmd/ayati/` contains the executable. Keep orchestration in `internal/app/`, the 20-step loop and shared message types in `internal/agent/`, Fireworks HTTP code in `internal/fireworks/`, direct host execution in `internal/shell/`, JSONL persistence in `internal/session/`, and terminal rendering in `internal/ui/`. Architectural decisions belong in `docs/`.

## Build, Test, and Development Commands

- `go run -buildvcs=false ./cmd/ayati` runs the harness in the current project.
- `go test -buildvcs=false ./...` runs all tests.
- `go test -buildvcs=false -race ./...` checks concurrent process and file code.
- `go vet -buildvcs=false ./...` runs static analysis.
- `CGO_ENABLED=0 go build -buildvcs=false -trimpath -o ayati ./cmd/ayati` builds the binary.
- `gofmt -w cmd internal` formats source files.

Run tests, vet, and the CGO-disabled build before handoff.

## Coding Style & Naming Conventions

Use idiomatic Go, tabs as produced by `gofmt`, short responsibility-focused packages, and contextual errors. Exported names use PascalCase; internal names use camelCase. Prefer concrete types and small consumer-owned interfaces. Keep every source file below 300 lines and avoid dependencies unless the project scope explicitly changes.

## Testing Guidelines

Colocate tests as `*_test.go`. Use `TestFeatureBehavior` names. Test the fixed 20-decision limit, Fireworks request shape, tool-call validation, shell timeout/output bounds, credential stripping, and JSONL round trips. Provider tests must use local HTTP transports; never require a real API key in automated tests.

## Architecture & Security

The model has exactly one tool: `shell`. Do not add providers, modes, catalogs, planners, snapshots, databases, compatibility layers, or background services without explicit approval. Shell execution is trusted-local and has the user's host permissions. Never log, persist, or pass `FIREWORKS_API_KEY` to shell children.

## Commits & Pull Requests

This directory currently has no Git history to inspect. Once Git is restored, use focused imperative commits such as `feat: add JSONL resume`. Pull requests should describe behavior, security implications, verification commands, and any user-visible terminal changes.
