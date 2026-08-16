# Ayati

Ayati is a small local-first coding agent for Linux. A user signs in with GitHub, creates a workspace from a repository and branch, lets Ayati initialize its dependencies inside one persistent Docker sandbox, works through one or more focused chat sessions, reviews the shared diff, and opens a draft pull request.

The controller is one Go process on your machine. Workspace metadata and complete conversations live in SQLite. There is no VM fleet, worker queue, Postgres server, or cloud orchestration layer.

## Requirements

- Linux
- Go 1.25 or newer
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

## GitHub App

Create a GitHub App for the local instance and give it these repository permissions:

- Contents: read and write
- Pull requests: read and write
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
2. Choose an installed repository and an existing branch, or create a working branch.
3. Create the workspace. Ayati clones the branch, starts its named sandbox, detects the dependency setup command, and runs that command inside the sandbox.
4. Use the original chat session or create another focused session in the same workspace. Each session keeps separate conversation and agent activity, while every session shares the repository, sandbox, branch, and uncommitted changes.
5. Discuss the task in chat. Discussion is durable but does not itself grant permission to edit. Send an explicit implementation request when the agent should inspect and modify the project using its single `shell(command)` tool.
6. Only one session can run the agent in a workspace at a time, preventing concurrent edits to the shared working tree.
7. Review workspace-wide Git status and the diff, provide a commit message and pull-request details, then create a draft pull request.
8. Stop the workspace when finished. This removes its container but preserves the cloned repository, sessions, conversations, and SQLite record.
9. Delete the workspace only when its local clone and complete session history are no longer needed. This does not delete its GitHub branch or pull request.

The default setup detection covers Go modules, npm/pnpm/Yarn lockfiles, and common Python project files. A workspace can supply an explicit setup command instead.

## Local data and security

- SQLite: `$XDG_CONFIG_HOME/ayati/ayati.db` or `~/.config/ayati/ayati.db`
- Fireworks config: `$XDG_CONFIG_HOME/ayati/config.json`
- GitHub user credential: `$XDG_CONFIG_HOME/ayati/github.json`
- cloned workspaces: `~/.local/share/ayati/workspaces`

The controller owns GitHub and Fireworks credentials. Git uses a temporary `GIT_ASKPASS` helper for authenticated clone and push; tokens are not placed in repository URLs, chat history, or sandbox environments.

Each active workspace gets one named Docker container. It runs as a non-root user with a read-only root filesystem, dropped capabilities, no-new-privileges, PID/memory/CPU bounds, and only that workspace mounted writable at `/workspace`. The Docker socket, host home, and controller credentials are not mounted. Network access remains enabled so dependency installation and project tests can work; this is a strong local boundary, not a complete hostile-code security system.

Workspace deletion is restricted to the managed data root. It removes the owned container and local workspace directory before cascading the workspace's sessions and messages from SQLite. Remote GitHub data is outside this action.

## Development

```bash
make test
make build
make check
```

`make check` verifies formatting, tests, race behavior, vet, and a CGO-disabled build. See [docs/architecture.md](docs/architecture.md) for component ownership and lifecycle details.
