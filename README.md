# Perpetual

Perpetual is a small local-first coding agent for Linux. A user signs in with GitHub, creates a workspace from an existing repository or a new GitHub project, lets Perpetual initialize its dependencies locally, works with the built-in coding agent, reviews the shared diff, and opens a draft pull request.

The controller is one long-lived Go process on your machine or personal server. Workspace metadata, accepted agent runs, and complete conversations live in SQLite. Closing the browser does not cancel accepted work; the browser reconnects through one lightweight event stream and reloads authoritative state. There is no VM fleet, worker queue, Postgres server, or cloud orchestration layer.

## Requirements

- Linux
- Go 1.25 or newer
- Node.js 22.12 or newer and npm (for interface development and builds)
- Git
- A GitHub App installed on the repositories Perpetual may access
- A Fireworks API key and model

Configure Fireworks from the terminal. The key is read without terminal echo and saved with private permissions:

```bash
go run -buildvcs=false ./cmd/perpetual config
```

Perpetual uses this single Fireworks connection for every agent run. Provider credentials and model selection are not exposed in the browser.

Install the locked React development dependencies once:

```bash
make web-install
```

## GitHub App

Create a GitHub App for the local instance and give it these repository permissions:

- Contents: read and write
- Pull requests: read and write
- Administration: read and write (required only when Perpetual creates a new repository)
- Metadata: read-only

Set its callback URL to `http://127.0.0.1:8080/auth/github/callback`, install it on the repositories you want to expose, then start Perpetual with its client credentials:

```bash
export PERPETUAL_GITHUB_CLIENT_ID="your-client-id"
export PERPETUAL_GITHUB_CLIENT_SECRET="your-client-secret"
make run
```

Open `http://127.0.0.1:8080`. A different callback or address can be supplied with `PERPETUAL_GITHUB_CALLBACK_URL` and `PERPETUAL_ADDRESS`.

## Compute environments

The Docker sandbox backend was removed. Setup and agent commands now run in a bounded local shell inside the managed workspace, and virtual-machine compute is planned as its replacement. The Environments interface in the sidebar is kept as client code for that future backend; until a compute provider is available it reports that environment management is unavailable.

## Workspace flow

1. Sign in through the GitHub App.
2. Choose an installed repository and create a new working branch, continue an existing branch, or explicitly work directly on a branch. New repositories are private by default, initialized with a README, and prepared on a new local working branch.
3. Optionally add write-only workspace environment variables. Mark only the values needed by dependency installation as available during setup.
4. Create the workspace. A live readiness screen follows clone, project analysis, dependency installation, baseline verification, and finalization. Perpetual encrypts its environment, deterministically records the project profile and Git baseline, creates requested working branches locally, and runs dependency setup in the writable workspace. If setup changes project files, those changes are recorded for review. If several applications are detected, preparation pauses for a project-root choice and continues after that choice.
5. Use the original chat session or create another focused session in the same workspace. Each session keeps separate conversation and agent activity, while every session shares the repository, branch, cache, and uncommitted changes. Every session uses the built-in Perpetual agent.
6. Discuss the task in chat. Discussion is durable but does not itself grant permission to edit. Send an explicit implementation request when the agent should inspect and modify the project. The model receives only `shell(command)` inside the prepared workspace.
7. Only one session can run the agent in a workspace at a time, preventing concurrent edits to the shared working tree. Use the composer Stop control to cancel that session's active model or shell work without discarding activity already recorded.
8. Review workspace-wide Git status and the diff, provide a commit message and pull-request details, then create a draft pull request. Pull-request publishing requires a working branch different from its base; direct branch work remains local until handled explicitly.
9. Stop the workspace when finished. This marks the workspace stopped while preserving the cloned repository, cache, sessions, conversations, and SQLite record. Resume returns the workspace to ready without rerunning dependency setup or rejecting preserved changes.
10. Delete the workspace only when its local clone and complete session history are no longer needed. This does not delete its GitHub branch or pull request.

Project analysis covers Go modules, npm/pnpm/Yarn projects, and common Python project files. It records the project root, runtimes, package managers, lockfiles, useful verification commands, manifest fingerprint, setup result, and baseline commit in SQLite. One nested project root is selected automatically; multiple roots stop with an explicit selection requirement instead of guessing. A workspace can supply an explicit setup command instead. Rust preparation is reported as unsupported.

Preparation progress, the selected project root, actionable failure stage, and configuration candidates are durable SQLite state. Reloading the browser therefore reconstructs the truthful readiness view instead of guessing from an in-memory task. Chat and additional sessions become available only after the workspace reaches `ready`; failed preparation can be retried without losing the clone or original session.

If Perpetual itself stops during repository preparation, the next startup marks that workspace as interrupted and offers an explicit retry or deletion. An accepted or running agent run is similarly marked interrupted and its session is marked failed; browser disconnection is recoverable, but a controller process restart cannot resume an in-memory provider or shell call.

For remote personal use, keep Perpetual on its default loopback address and reach it through an authenticated HTTPS reverse proxy, VPN, or SSH tunnel. Do not publish the raw HTTP port directly to the internet. The current authentication and credential store are designed for one user, not a multi-tenant deployment.

## Local data and security

- SQLite: `$XDG_CONFIG_HOME/perpetual/perpetual.db` or `~/.config/perpetual/perpetual.db`
- workspace environment key: `$XDG_CONFIG_HOME/perpetual/environment.key` or `~/.config/perpetual/environment.key`
- private Fireworks config: `$XDG_CONFIG_HOME/perpetual/config.json`
- GitHub user credential: `$XDG_CONFIG_HOME/perpetual/github.json`
- cloned workspaces: `~/.local/share/perpetual/workspaces`

The controller owns GitHub and Fireworks credentials. Git uses a temporary `GIT_ASKPASS` helper for authenticated clone and push; tokens are not placed in repository URLs, chat history, or shell environments.

Workspace environment values are encrypted in SQLite with a private local key. APIs return names and scope but never stored values. Values are supplied only to the bounded shell that runs each setup or agent command, are not stored in the repository, and are best-effort redacted from captured output. A command that is allowed to use a value can still read or transmit it; use narrowly scoped development credentials, especially because workspace network access is enabled.

Back up `perpetual.db` and `environment.key` together. Perpetual refuses to replace a missing key for a database that already uses encrypted workspace environments.

Setup and agent commands run on the controller machine in a bounded local shell with a two-minute timeout, 64 KiB command limit, and 32 KiB per-stream output bounds. Each workspace gets a private home directory and tool caches under its managed cache path, so host configuration and controller credentials are never visible to commands. Tool caches survive Stop and Resume. This is an intermediate execution mode while virtual-machine compute is designed; it runs with host user privileges and is not a hostile-code sandbox.

The controller owns GitHub and Fireworks credentials. Authenticated clone and push use host Git with a short-lived private askpass helper; tokens are never exposed to shell commands.Workspace deletion is restricted to the managed data root. It removes the local workspace directory (including the tool cache) before cascading the workspace's sessions and messages from SQLite. Remote GitHub data is outside this action.

New-project creation is also controller-owned. Perpetual creates the remote repository before recording and preparing the local workspace, and never automatically deletes that repository if a later local step fails. The error names the created repository so the user can retry or manage it explicitly.

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
