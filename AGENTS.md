# Repository Guidelines

## Project scope

Ayati is a small local-first Go coding agent for one Linux machine. Its product flow is GitHub App login, repository and branch selection, SQLite-backed workspace creation, one persistent Docker sandbox per active workspace, dependency initialization, durable browser chat, explicit agent work, diff review, and a draft pull request.

Keep this boundary small. Do not add providers, Postgres, virtual machines, queues, worker fleets, multi-user tenancy, planners, or compatibility layers without explicit approval. The model has one tool: `shell(command)`.

## Package ownership

- `cmd/ayati/`: process entry point and signal setup only.
- `internal/webapp/`: local HTTP server, routes, embedded UI, and component wiring.
- `internal/workspace/`: SQLite state, lifecycle, trusted Git, setup, review, and publish.
- `internal/sandbox/`: persistent Docker containers and bounded shell execution.
- `internal/githubapp/`: GitHub user authentication and repository operations.
- `internal/chat/`: durable workspace conversation and serialized agent runs.
- `internal/agent/`: prompt, shared messages, and sequential loop.
- `internal/config/`: private Fireworks configuration and setup command.
- `internal/fireworks/`: Fireworks HTTP request and response handling.
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

The controller owns GitHub, Git, SQLite, Docker lifecycle, and Fireworks credentials. Never expose credentials to the model sandbox, repository URLs, messages, logs, or tests.

Sandbox containers run non-root with a read-only root, dropped capabilities, no-new-privileges, bounded resources, private temporary/home mounts, and only the selected workspace writable. Preserve command, output, timeout, workspace, and process-group cancellation bounds. Validate container names before lifecycle actions. Network access is currently allowed for dependency setup.

Each workspace keeps one named sandbox until the user stops it. Discussion must not modify files until the user explicitly authorizes agent work. Git commits, pushes, and pull requests remain controller-owned actions initiated from the UI.

## Git changes

Keep commits focused and use short imperative messages such as `feat: add workspace setup`. Do not mix unrelated cleanup with behavior changes.
