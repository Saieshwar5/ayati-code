# Repository Guidelines

## Project scope

Ayati is a small local-first Go coding agent for one Linux machine. Its product flow is GitHub App login, repository and branch selection, SQLite-backed workspace creation, one persistent Docker sandbox per active workspace, dependency initialization, reusable global agent profiles and Markdown skills, durable browser chat, explicit agent work, diff review, and a draft pull request.

Keep this boundary small. Do not add providers, Postgres, virtual machines, queues, worker fleets, multi-user tenancy, planners, or compatibility layers without explicit approval. The model has one tool: `shell(command)`.

## Package ownership

- `cmd/ayati/`: process entry point and signal setup only.
- `internal/database/`: the shared SQLite connection and connection-level safety configuration.
- `internal/environment/`: reusable compute definitions, availability, exclusive workspace leases, and lease/runtime coordination.
- `internal/webapp/`: local HTTP server, routes, embedded UI, and component wiring.
- `internal/workspace/`: SQLite state, lifecycle, deterministic project preparation, trusted Git, review, and publish.
- `internal/sandbox/`: persistent Docker containers, the verified Docker environment driver, and bounded shell execution.
- `internal/githubapp/`: GitHub user authentication and repository operations.
- `internal/chat/`: durable workspace conversation and serialized agent runs.
- `internal/agent/`: agent and skill definitions, prompt composition, shared messages, and sequential loop.
- `internal/provider/`: provider definitions, registration, discovery, and runtime resolution.
- `internal/config/`: versioned private provider configuration and setup command.
- `internal/fireworks/`: Fireworks protocol adapter.
- `internal/openaichat/`: shared OpenAI-compatible chat protocol and connection verification.
- `docs/`: architecture and important design decisions.

Keep logic in the package that owns the responsibility. `internal/webapp` may connect packages, but infrastructure packages should not depend on it.

## Development commands

```bash
make sandbox
make run
make test
make check
```

Before handoff, run formatting, tests, race checks, vet, and the CGO-disabled build.

## Code and tests

Use idiomatic Go and `gofmt`. Prefer concrete types, small consumer-owned interfaces, contextual errors, and short responsibility-focused files. Keep source files below 300 lines. Exported names use PascalCase; unexported names use camelCase.

Colocate tests as `*_test.go` and name them `TestFeatureBehavior`. Cover changed behavior and failure paths. External API tests use injected HTTP transports and never require a real key, network connection, Docker daemon, or GitHub account. HTTP handler tests use `httptest` recorders.

## Security and runtime rules

The controller owns GitHub, Git, SQLite, environment leases, Docker lifecycle, and model-provider credentials. Never expose credentials to the model sandbox, repository URLs, messages, logs, or tests.

Workspace environment values are separate user-provided development credentials. Keep them encrypted at rest, write-only through the API, out of Git and Docker metadata, and best-effort redacted from shell results. Do not confuse them with controller-owned GitHub or model-provider credentials. A sandbox command that receives a workspace value can read it; do not claim otherwise.

Sandbox containers run non-root with a read-only root, dropped capabilities, no-new-privileges, bounded resources, private temporary/home mounts, and a managed cache outside the repository. Explore mounts the selected workspace read-only; Develop mounts it read-write. Preserve command, output, timeout, workspace, cache, mount verification, and process-group cancellation bounds. Validate container names before lifecycle actions. Network access is currently allowed for dependency setup.

Each workspace keeps one named sandbox until the user stops it. Custom agent instructions and inert Markdown skills never override workspace authority or controller credential and publishing rules. Skills do not add tools or executable controller behavior. Discussion must not modify files until the user explicitly authorizes agent work. Git commits, pushes, and pull requests remain controller-owned actions initiated from the UI.

## Git changes

Keep commits focused and use short imperative messages such as `feat: add workspace setup`. Do not mix unrelated cleanup with behavior changes.
