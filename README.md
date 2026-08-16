# Ayati

Ayati is a small local-first coding agent for Linux. A user signs in with GitHub, creates a workspace from an existing repository or a new GitHub project, lets Ayati initialize its dependencies inside one persistent Docker sandbox, works through one or more focused chat sessions, reviews the shared diff, and opens a draft pull request.

The controller is one Go process on your machine. Workspace metadata and complete conversations live in SQLite. There is no VM fleet, worker queue, Postgres server, or cloud orchestration layer.

## Requirements

- Linux
- Go 1.25 or newer
- Node.js 22.12 or newer and npm (for interface development and builds)
- Git
- Docker Engine
- A GitHub App installed on the repositories Ayati may access
- A Fireworks API key and model

Build the local sandbox image once:

```bash
make sandbox
```

Configure the model provider. The key is read without terminal echo and saved with private permissions:

```bash
go run -buildvcs=false ./cmd/ayati config
```

Install the locked React development dependencies once:

```bash
make web-install
```

## GitHub App

Create a GitHub App for the local instance and give it these repository permissions:

- Contents: read and write
- Pull requests: read and write
- Administration: read and write (required only when Ayati creates a new repository)
- Metadata: read-only

Set its callback URL to `http://127.0.0.1:8080/auth/github/callback`, install it on the repositories you want to expose, then start Ayati with its client credentials:

```bash
export AYATI_GITHUB_CLIENT_ID="your-client-id"
export AYATI_GITHUB_CLIENT_SECRET="your-client-secret"
make run
```

Open `http://127.0.0.1:8080`. A different callback or address can be supplied with `AYATI_GITHUB_CALLBACK_URL` and `AYATI_ADDRESS`.

## Workspace flow

1. Sign in through the GitHub App.
2. Choose an installed repository, or create a personal GitHub repository from Ayati. New repositories are private by default and initialized with a README. Choose the workspace authority; Explore is the protected default, while Develop additionally asks for a local working branch.
3. Optionally add write-only workspace environment variables. Mark only the values needed by dependency installation as available during setup.
4. Create the workspace. A live readiness screen follows clone, project analysis, dependency installation, baseline verification, and authority sealing. Ayati encrypts its environment, deterministically records the project profile and clean Git commit, creates new working branches locally, and runs dependency setup in a trusted writable initialization phase. If several applications are detected, preparation pauses for a project-root choice and continues after that choice. Explore is sealed only when setup leaves tracked and non-ignored project files unchanged, then recreated with `/workspace` read-only before the agent can run.
5. Use the original chat session or create another focused session in the same workspace. Each session keeps separate conversation and agent activity, while every session shares the repository, sandbox, branch, environment, and uncommitted changes.
6. Discuss the task in chat. Discussion is durable but does not itself grant permission to edit. Send an explicit implementation request when the agent should inspect and modify the project using its single `shell(command)` tool.
7. Only one session can run the agent in a workspace at a time, preventing concurrent edits to the shared working tree.
8. Change authority from the Workspace inspector when needed. Explore to Develop creates a local working branch when the workspace is still on its base branch, then remounts the project read-write. Develop to Explore preserves current modifications and remounts them read-only. Authority changes are rejected while an agent is working.
9. In Develop, review workspace-wide Git status and the diff, provide a commit message and pull-request details, then create a draft pull request. Explore rejects publishing.
10. Stop the workspace when finished. This removes its container but preserves the cloned repository, environment, sessions, conversations, and SQLite record.
11. Delete the workspace only when its local clone and complete session history are no longer needed. This does not delete its GitHub branch or pull request.

Project analysis covers Go modules, npm/pnpm/Yarn projects, and common Python project files. It records the project root, runtimes, package managers, lockfiles, useful verification commands, manifest fingerprint, setup result, and baseline commit in SQLite. One nested project root is selected automatically; multiple roots stop with an explicit selection requirement instead of guessing. A workspace can supply an explicit setup command instead. Rust preparation is reported as unsupported until the sandbox includes a compatible Rust toolchain.

Preparation progress, the selected project root, actionable failure stage, and configuration candidates are durable SQLite state. Reloading the browser therefore reconstructs the truthful readiness view instead of guessing from an in-memory task. Chat and additional sessions become available only after the workspace reaches `ready`; failed preparation can be retried without losing the clone or original session.

## Local data and security

- SQLite: `$XDG_CONFIG_HOME/ayati/ayati.db` or `~/.config/ayati/ayati.db`
- workspace environment key: `$XDG_CONFIG_HOME/ayati/environment.key` or `~/.config/ayati/environment.key`
- Fireworks config: `$XDG_CONFIG_HOME/ayati/config.json`
- GitHub user credential: `$XDG_CONFIG_HOME/ayati/github.json`
- cloned workspaces: `~/.local/share/ayati/workspaces`

The controller owns GitHub and Fireworks credentials. Git uses a temporary `GIT_ASKPASS` helper for authenticated clone and push; tokens are not placed in repository URLs, chat history, or sandbox environments.

Workspace environment values are encrypted in SQLite with a private local key. APIs return names and scope but never stored values. Values are sent through standard input to a short-lived sandbox launcher for each shell command, are not stored in the repository or permanent Docker configuration, and are best-effort redacted from captured output. A command that is allowed to use a value can still read or transmit it; use narrowly scoped development credentials, especially because workspace network access is enabled.

Back up `ayati.db` and `environment.key` together. Ayati refuses to replace a missing key for a database that already uses encrypted workspace environments.

Each active workspace gets one named Docker container. It runs as a non-root user with a read-only root filesystem, dropped capabilities, no-new-privileges, PID/memory/CPU bounds, and the selected repository mounted at `/workspace`. Explore mounts it read-only; Develop mounts it read-write. Writable `/tmp` and `/home/ayati` tmpfs locations remain available in both authorities. `/cache` is a separate writable bind under the managed workspace directory, so language and package-manager caches survive container recreation without making the repository writable. Ayati inspects the effective repository and cache mounts and replaces a container whose configuration does not match the controller. The Docker socket, host home, and controller credentials are not mounted. Network access remains enabled so dependency installation and project tests can work; this is a strong local boundary, not a complete hostile-code security system.

Workspace deletion is restricted to the managed data root. It removes the owned container and local workspace directory before cascading the workspace's sessions and messages from SQLite. Remote GitHub data is outside this action.

New-project creation is also controller-owned. Ayati creates the remote repository before recording and preparing the local workspace, and never automatically deletes that repository if a later local step fails. The error names the created repository so the user can retry or manage it explicitly.

## Development

The browser interface lives in `web/` and uses React, TypeScript, and Vite. Vite writes the
production bundle to `internal/webapp/dist/`; that bundle is committed and embedded into the Go
binary, so production still runs as one local process with no Node.js server.

```bash
make web-check
make test
make build
make check
```

`make check` type-checks, tests, and builds the React interface, then verifies Go formatting,
tests, race behavior, vet, and a CGO-disabled build. See
[docs/architecture.md](docs/architecture.md) for component ownership and lifecycle details.
