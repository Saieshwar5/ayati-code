# Repository Guidelines

## Project scope

Perpetual is a controller-led Go coding agent. Its product flow is GitHub App login, repository and branch selection, SQLite-backed workspace creation, dependency initialization through a bounded shell contract, durable browser chat (currently parked while a new agent is designed), execution-room agent work, diff review, and a draft pull request.

Keep the control plane small. The approved cloud direction uses AWS Lambda MicroVMs behind the existing `internal/workspaceruntime.Runtime` seam and a bounded in-process durable worker model. Do not add Postgres, Kafka/Temporal, external queues, a separate worker fleet, or a full multi-tenant SaaS control plane without explicit approval. The planned agent has one tool: `shell(command)` through `internal/exec`. A future self-managed Firecracker provider is a separate later decision, not implied by the Lambda MicroVMs approval.

## Package ownership

- `cmd/perpetual/`: process entry point and signal setup only.
- `internal/database/`: the shared SQLite connection and connection-level safety configuration.
- `internal/exec/`: bounded shell execution and the shell contract for setup, preparation, and the planned agent. The local shell is one implementation of this contract.
- `internal/workspaceruntime/`: the workspace runtime contract, the local development adapter, and the cloud/Lambda MicroVMs provider implementation.
- `internal/webapp/`: HTTP routes, embedded UI, control-plane wiring, and routing between the controller blocks.
- `internal/workspace/`: SQLite state, lifecycle, sessions and stored conversation messages, deterministic project preparation, trusted Git, review, and publish.
- `internal/githubapp/`: GitHub user authentication and repository operations.
- `docs/`: architecture and important design decisions.

Keep logic in the package that owns the responsibility. `internal/webapp` may connect packages, but infrastructure packages should not depend on it.

## Development commands

```bash
make run
make test
make check
```

Before handoff, run formatting, tests, race checks, vet, and the CGO-disabled build.

## Code and tests

Use idiomatic Go and `gofmt`. Prefer concrete types, small consumer-owned interfaces, contextual errors, and short responsibility-focused files. Keep source files below 300 lines. Exported names use PascalCase; unexported names use camelCase.

Colocate tests as `*_test.go` and name them `TestFeatureBehavior`. Cover changed behavior and failure paths. External API tests use injected HTTP transports and never require a real key, network connection, Docker daemon, or GitHub account. HTTP handler tests use `httptest` recorders.

## Security and runtime rules

The controller owns GitHub, Git, SQLite, scheduling, and workspace execution. Never expose credentials to shell commands, repository URLs, messages, logs, or tests. AWS credentials and AWS API auth tokens remain controller-only.

Workspace environment values are separate user-provided development credentials. Keep them encrypted at rest, write-only through the API, and best-effort redacted from shell results. A shell command that receives a workspace value can read it; do not claim otherwise.

Setup and agent commands run through the bounded shell contract in `internal/exec`. Preserve command, output, timeout, working-directory, environment, and process-group cancellation bounds. The local runtime uses a private per-workspace `HOME` and managed tool caches; the cloud runtime executes the same contract inside a Lambda MicroVM and receives workspace values only over the authenticated data plane. The controller never places GitHub or AWS control-plane credentials in the shell environment.

The Fireworks agent backend was removed; the browser chat UI is parked until the new execution-room agent is built. When it lands, the agent must never override controller credential and publishing rules; discussion must not modify files until the user explicitly authorizes agent work. Execution-room loops must be durable, user-scoped, bounded, and cancelable. Git commits, pushes, and pull requests remain controller-owned actions initiated from the UI.

## Git changes

Keep commits focused and use short imperative messages such as `feat: add workspace setup`. Do not mix unrelated cleanup with behavior changes.
