# Ayati

Ayati is a small local-first coding agent for Linux. A user signs in with GitHub, creates a workspace from an existing repository or a new GitHub project, lets Ayati initialize its dependencies inside an exclusively leased local Docker environment, selects a built-in or custom agent for each chat session, reviews the shared diff, and opens a draft pull request.

The controller is one long-lived Go process on your machine or personal server. Workspace metadata, accepted agent runs, and complete conversations live in SQLite. Closing the browser does not cancel accepted work; the browser reconnects through one lightweight event stream and reloads authoritative state. There is no VM fleet, worker queue, Postgres server, or cloud orchestration layer.

## Requirements

- Linux
- Go 1.25 or newer
- Node.js 22.12 or newer and npm (for interface development and builds)
- Git
- Docker Engine
- A GitHub App installed on the repositories Ayati may access
- An API key and model for at least one supported provider

Build the local sandbox image once:

```bash
make sandbox
```

Optionally configure Fireworks from the terminal. The key is read without terminal echo and saved with private permissions:

```bash
go run -buildvcs=false ./cmd/ayati config
```

Fireworks, OpenAI, OpenRouter, Groq, Together AI, and DeepSeek can be configured from **Agent Studio → Providers** after Ayati starts. The browser never receives a saved API key; leaving the key field blank preserves the existing value. Configured OpenAI-compatible providers expose an on-demand model catalog in provider and custom-agent forms, while manual model IDs always remain available. Fireworks currently uses manual model entry because its catalog API requires additional account context.

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

## Environment capacity

Ayati creates one **Local Docker** environment on first startup. Open **Environments** in the sidebar to see available and occupied capacity, add another local Docker environment, repair failed image resolution, or delete unused capacity. Each environment configures a local image reference, CPU, memory, process, and network limits. The image must already exist in the local Docker engine; Ayati resolves and stores its immutable image identity before making the environment available.

An occupied environment shows the workspace holding its exclusive lease and cannot be deleted. Stop that workspace first. Image-provisioning failures remain visible until Repair succeeds or the unused environment is deleted. A runtime failure is stricter: the capacity remains quarantined until its failed workspace is safely deleted, preventing uncertain compute from being reused or forgotten.

## Workspace flow

1. Sign in through the GitHub App.
2. Choose an installed repository, or create a personal GitHub repository from Ayati. New repositories are private by default and initialized with a README. Choose the workspace authority; Explore is the protected default, while Develop additionally asks for a local working branch.
3. Optionally add write-only workspace environment variables. Mark only the values needed by dependency installation as available during setup.
4. Create the workspace. A live readiness screen follows clone, project analysis, dependency installation, baseline verification, and authority sealing. Ayati encrypts its environment, deterministically records the project profile and clean Git commit, creates new working branches locally, and runs dependency setup in a trusted writable initialization phase. If several applications are detected, preparation pauses for a project-root choice and continues after that choice. Explore is sealed only when setup leaves tracked and non-ignored project files unchanged, then recreated with `/workspace` read-only before the agent can run.
5. Use the original chat session or create another focused session in the same workspace. Each session keeps separate conversation and agent activity, while every session shares the repository, leased environment, branch, cache, and uncommitted changes. New sessions start with the current global default agent.
6. Open Agent Studio to configure a supported provider and create reusable agents with their own identity, provider, model, instructions, step budget, shell capability, and ordered Markdown skills. OpenAI-compatible connections can be verified and their available models browsed on demand. The built-in Ayati agent is protected, exactly one global default always exists, and changing the default does not rewrite existing session selections.
7. Create or import reusable skills from `.md` files, then attach them to editable custom agents. Skills are inert prompt guidance: they cannot add tools or override workspace, credential, and publishing rules. Attached skills must be detached before archival.
8. Select an available agent above the chat composer. Agents do not keep their own conversation state: the selected agent receives the current workspace facts and that session's history for each run. Assistant messages retain the agent identity and attached skill revisions even when their definitions later change.
9. Discuss the task in chat. Discussion is durable but does not itself grant permission to edit. Send an explicit implementation request when the agent should inspect and modify the project. Agents with shell capability disabled receive no model-facing tool; enabled agents receive only `shell(command)` under the workspace's Explore or Develop authority.
10. Only one session can run an agent in a workspace at a time, preventing concurrent edits to the shared working tree. Use the composer Stop control to cancel that session's active provider or shell work without discarding activity already recorded.
11. Change authority from the Workspace inspector when needed. Explore to Develop creates a local working branch when the workspace is still on its base branch, then remounts the project read-write. Develop to Explore preserves current modifications and remounts them read-only. Authority changes are rejected while an agent is working.
12. In Develop, review workspace-wide Git status and the diff, provide a commit message and pull-request details, then create a draft pull request. Explore rejects publishing.
13. Stop the workspace when finished. This destroys its disposable runtime and releases the environment for another workspace, while preserving the cloned repository, cache, sessions, conversations, and SQLite record. Resume acquires available environment capacity and creates a runtime with the stored authority; it does not rerun dependency setup or reject preserved Develop changes.
14. Delete the workspace only when its local clone and complete session history are no longer needed. This does not delete its GitHub branch or pull request.

Project analysis covers Go modules, npm/pnpm/Yarn projects, and common Python project files. It records the project root, runtimes, package managers, lockfiles, useful verification commands, manifest fingerprint, setup result, and baseline commit in SQLite. One nested project root is selected automatically; multiple roots stop with an explicit selection requirement instead of guessing. A workspace can supply an explicit setup command instead. Rust preparation is reported as unsupported until the sandbox includes a compatible Rust toolchain.

Preparation progress, the selected project root, actionable failure stage, and configuration candidates are durable SQLite state. Reloading the browser therefore reconstructs the truthful readiness view instead of guessing from an in-memory task. Chat and additional sessions become available only after the workspace reaches `ready`; failed preparation can be retried without losing the clone or original session.

If Ayati itself stops during repository preparation, the next startup marks that workspace as interrupted, cleans up its acquiring or active runtime, and offers an explicit retry or deletion. An accepted or running agent run is similarly marked interrupted and its session is marked failed; browser disconnection is recoverable, but a controller process restart cannot resume an in-memory provider or shell call. Ready workspaces restore and verify their active leased runtime. If capacity is no longer available, the workspace becomes stopped with an actionable message instead of making another environment assignment implicitly.

For remote personal use, keep Ayati on its default loopback address and reach it through an authenticated HTTPS reverse proxy, VPN, or SSH tunnel. Do not publish the raw HTTP port directly to the internet. The current authentication and credential store are designed for one user, not a multi-tenant deployment.

## Local data and security

- SQLite: `$XDG_CONFIG_HOME/ayati/ayati.db` or `~/.config/ayati/ayati.db`
- workspace environment key: `$XDG_CONFIG_HOME/ayati/environment.key` or `~/.config/ayati/environment.key`
- private model-provider config: `$XDG_CONFIG_HOME/ayati/config.json`
- GitHub user credential: `$XDG_CONFIG_HOME/ayati/github.json`
- cloned workspaces: `~/.local/share/ayati/workspaces`

The controller owns GitHub and model-provider credentials. Git uses a temporary `GIT_ASKPASS` helper for authenticated clone and push; tokens are not placed in repository URLs, chat history, or sandbox environments.

Workspace environment values are encrypted in SQLite with a private local key. APIs return names and scope but never stored values. Values are sent through standard input to a short-lived sandbox launcher for each shell command, are not stored in the repository or permanent Docker configuration, and are best-effort redacted from captured output. A command that is allowed to use a value can still read or transmit it; use narrowly scoped development credentials, especially because workspace network access is enabled.

Back up `ayati.db` and `environment.key` together. Ayati refuses to replace a missing key for a database that already uses encrypted workspace environments.

On first startup Ayati registers one ready Local Docker environment from the configured sandbox image. Each active workspace exclusively leases one ready environment, and the controller creates a disposable runtime identified by the environment and lease generation. It runs as a non-root user with a read-only root filesystem, dropped capabilities, no-new-privileges, PID/memory/CPU bounds, and the selected repository mounted at `/workspace`. Explore mounts it read-only; Develop mounts it read-write. Writable `/tmp` and `/home/ayati` tmpfs locations remain available in both authorities. `/cache` is a separate writable bind under the managed workspace directory, so language and package-manager caches survive runtime replacement and environment reassignment without making the repository writable. Ayati verifies the runtime's exact lease identity and effective mounts before shell use or deletion. A failed or uncertain runtime quarantines its environment rather than making that capacity available. The Docker socket, host home, and controller credentials are not mounted. Network access remains enabled so dependency installation and project tests can work; this is a strong local boundary, not a complete hostile-code security system.

Workspace deletion is restricted to the managed data root. It first proves the workspace has no live or uncertain runtime, then removes the local workspace directory before cascading the workspace's sessions and messages from SQLite. Remote GitHub data is outside this action.

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
