# Perpetual

Perpetual is a small local-first coding agent for Linux. A user signs in with GitHub, creates a workspace from an existing repository or a new GitHub project, lets Perpetual initialize its dependencies locally, reviews the shared diff, and opens a draft pull request. A new built-in agent is planned; the current agent backend and its model provider were removed but the chat interface is kept for that future agent.

The controller is one long-lived Go process on your machine or personal server. Workspace metadata, sessions, and complete conversations live in SQLite. The browser reconnects through one lightweight event stream and reloads authoritative state. There is no VM fleet, worker queue, Postgres server, or cloud orchestration layer.

## Requirements

- Linux
- Go 1.25 or newer
- Node.js 22.12 or newer and npm (for interface development and builds)
- Git
- A GitHub App installed on the repositories Perpetual may access

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

Set its callback URL, install it on the repositories you want to expose, then start Perpetual with its client credentials:

```bash
export PERPETUAL_GITHUB_CLIENT_ID="your-client-id"
export PERPETUAL_GITHUB_CLIENT_SECRET="your-client-secret"
make run
```

Open `http://127.0.0.1:8080`. The callback URL must be the exact address the browser reaches after GitHub sign-in:

- Local use: `http://127.0.0.1:8080/auth/github/callback` (the default).
- Server use: your public URL, for example `https://perpetual.example.com/auth/github/callback`. Set it in the GitHub App settings **and** start Perpetual with `PERPETUAL_GITHUB_CALLBACK_URL` (or `PERPETUAL_PUBLIC_URL`, see below).

## Run Perpetual on a server and open it from other devices

Perpetual is one Go binary that serves its own UI and API, so it runs on any Linux server. To reach it from your laptop or other devices you make the server's port reachable and tell Perpetual (and GitHub) the public address. Four options, in order of preference:

### Option A: HTTPS reverse proxy (recommended for a permanent server)

Keep Perpetual bound to the loopback address and let a small HTTPS reverse proxy (Caddy or nginx) terminate TLS and forward to it. This gives you a normal `https://` URL, automatic certificates with Caddy, and no raw HTTP port exposed.

1. Start Perpetual on the server with its public URL:

   ```bash
   export PERPETUAL_PUBLIC_URL="https://perpetual.example.com"
   make run    # still listens on 127.0.0.1:8080
   ```

2. Set the GitHub App callback URL to `https://perpetual.example.com/auth/github/callback`.

3. Put Caddy in front (Caddyfile):

   ```
   perpetual.example.com {
       reverse_proxy 127.0.0.1:8080
   }
   ```

   Point the domain's DNS A record at the server, then `caddy run`. Caddy obtains and renews the TLS certificate automatically. nginx works the same way with `proxy_pass http://127.0.0.1:8080;`.

4. Open `https://perpetual.example.com` from any device.

`PERPETUAL_PUBLIC_URL` only tells GitHub where to redirect the browser after sign-in; it does not change where Perpetual listens. `PERPETUAL_ADDRESS` or `-address` still controls the bind address.

### Option B: built-in TLS (no reverse proxy)

Perpetual can serve HTTPS directly with a certificate and key:

```bash
export PERPETUAL_ADDRESS="0.0.0.0:8443"
export PERPETUAL_TLS_CERT="/etc/perpetual/cert.pem"
export PERPETUAL_TLS_KEY="/etc/perpetual/key.pem"
export PERPETUAL_PUBLIC_URL="https://perpetual.example.com:8443"
make run
```

Open your firewall to TCP 8443, set the GitHub App callback URL to `https://perpetual.example.com:8443/auth/github/callback`, and open `https://perpetual.example.com:8443` from any device. The certificate must match the hostname; obtain one from a certificate authority (or use Caddy's automatic certificates from Option A instead).

### Option C: SSH tunnel (quick, no public exposure)

For occasional access with nothing exposed on the internet, forward the port over SSH. On your laptop:

```bash
ssh -N -L 8080:127.0.0.1:8080 user@server
```

Then open `http://127.0.0.1:8080` on the laptop. The GitHub App callback URL stays at `http://127.0.0.1:8080/auth/github/callback` because the browser talks to the tunnel's local endpoint. This works from anywhere you can SSH to the server.

### Option D: VPN

Put the laptop and server on the same private network (Tailscale, WireGuard, or similar), bind Perpetual to the server's private IP with `PERPETUAL_ADDRESS="100.x.y.z:8080"`, and open `http://100.x.y.z:8080` from the laptop. Keep the GitHub App callback URL on the same private address.

### Protecting a public server with a password

When Perpetual is reachable beyond your own machine (Options A and B), add a password gate so a stranger cannot take over your GitHub session (any username works, the password must match):

```bash
export PERPETUAL_ACCESS_PASSWORD="a-long-random-password"
make run
```

The browser asks for the password once per visit. The same gate covers every request including the live event stream, and it is enforced before the GitHub App sign-in. Protect the value like any credential: prefer `systemd` `EnvironmentFile` or your process manager's secret handling rather than shell history. HTTPS (Option A or B) is still required so the password is not sent in the clear.

### What changed for server use

- `PERPETUAL_PUBLIC_URL` / `-public-url`: the externally visible URL used to derive the GitHub callback URL.
- `PERPETUAL_ADDRESS` / `-address`: the listen address (existing). Use `0.0.0.0` only when you intend to expose the port; the GitHub App callback can no longer default to a wildcard address, so a non-loopback listen address requires `PERPETUAL_PUBLIC_URL` or `PERPETUAL_GITHUB_CALLBACK_URL`.
- `PERPETUAL_TLS_CERT` / `PERPETUAL_TLS_KEY` / `-tls-cert` / `-tls-key`: serve HTTPS directly (both files required together).
- `PERPETUAL_ACCESS_PASSWORD` / `-access-password`: optional HTTP Basic password gate for remote access.

Run `perpetual -h` for the full flag list.

## Compute environments

The Docker sandbox backend was removed. Setup and agent commands now run in a bounded local shell inside the managed workspace, and virtual-machine compute is planned as its replacement. The Environments interface in the sidebar is kept as client code for that future backend; until a compute provider is available it reports that environment management is unavailable. The Fireworks-backed agent was also removed; the chat, sessions, and composer interface are kept as client code for the new agent, so sending a message currently reports that the agent is unavailable.

## Workspace flow

1. Sign in through the GitHub App.
2. Choose an installed repository and create a new working branch, continue an existing branch, or explicitly work directly on a branch. New repositories are private by default, initialized with a README, and prepared on a new local working branch.
3. Optionally add write-only workspace environment variables. Mark only the values needed by dependency installation as available during setup.
4. Create the workspace. A live readiness screen follows clone, project analysis, dependency installation, baseline verification, and finalization. Perpetual encrypts its environment, deterministically records the project profile and Git baseline, creates requested working branches locally, and runs dependency setup in the writable workspace. If setup changes project files, those changes are recorded for review. If several applications are detected, preparation pauses for a project-root choice and continues after that choice.
5. Use the original chat session or create another focused session in the same workspace. Each session keeps separate conversation history, while every session shares the repository, branch, cache, and uncommitted changes. The chat composer is currently disabled while a new agent is built; stored conversation history remains readable.
8. Review workspace-wide Git status and the diff, provide a commit message and pull-request details, then create a draft pull request. Pull-request publishing requires a working branch different from its base; direct branch work remains local until handled explicitly.
9. Stop the workspace when finished. This marks the workspace stopped while preserving the cloned repository, cache, sessions, conversations, and SQLite record. Resume returns the workspace to ready without rerunning dependency setup or rejecting preserved changes.
10. Delete the workspace only when its local clone and complete session history are no longer needed. This does not delete its GitHub branch or pull request.

Project analysis covers Go modules, npm/pnpm/Yarn projects, and common Python project files. It records the project root, runtimes, package managers, lockfiles, useful verification commands, manifest fingerprint, setup result, and baseline commit in SQLite. One nested project root is selected automatically; multiple roots stop with an explicit selection requirement instead of guessing. A workspace can supply an explicit setup command instead. Rust preparation is reported as unsupported.

Preparation progress, the selected project root, actionable failure stage, and configuration candidates are durable SQLite state. Reloading the browser therefore reconstructs the truthful readiness view instead of guessing from an in-memory task. Additional sessions become available after the workspace reaches `ready`; failed preparation can be retried without losing the clone or original session.

If Perpetual itself stops during repository preparation, the next startup marks that workspace as interrupted and offers an explicit retry or deletion.

For remote personal use, prefer an authenticated HTTPS reverse proxy, VPN, or SSH tunnel (see "Run Perpetual on a server" above). If you publish the HTTP port directly, you must use HTTPS (built-in TLS or a proxy) and set a strong `PERPETUAL_ACCESS_PASSWORD`; the current authentication and credential store are designed for one user, not a multi-tenant deployment.

## Local data and security

- SQLite: `$XDG_CONFIG_HOME/perpetual/perpetual.db` or `~/.config/perpetual/perpetual.db`
- workspace environment key: `$XDG_CONFIG_HOME/perpetual/environment.key` or `~/.config/perpetual/environment.key`
- GitHub user credential: `$XDG_CONFIG_HOME/perpetual/github.json`
- cloned workspaces: `~/.local/share/perpetual/workspaces`

Git uses a temporary `GIT_ASKPASS` helper for authenticated clone and push; tokens are not placed in repository URLs, chat history, or shell environments.

Workspace environment values are encrypted in SQLite with a private local key. APIs return names and scope but never stored values. Values are supplied only to the bounded shell that runs each setup or agent command, are not stored in the repository, and are best-effort redacted from captured output. A command that is allowed to use a value can still read or transmit it; use narrowly scoped development credentials, especially because workspace network access is enabled.

Back up `perpetual.db` and `environment.key` together. Perpetual refuses to replace a missing key for a database that already uses encrypted workspace environments.

Setup and agent commands run on the controller machine in a bounded local shell with a two-minute timeout, 64 KiB command limit, and 32 KiB per-stream output bounds. Each workspace gets a private home directory and tool caches under its managed cache path, so host configuration and controller credentials are never visible to commands. Tool caches survive Stop and Resume. This is an intermediate execution mode while virtual-machine compute is designed; it runs with host user privileges and is not a hostile-code sandbox.

Authenticated clone and push use host Git with a short-lived private askpass helper; tokens are never exposed to shell commands. Workspace deletion is restricted to the managed data root. It removes the local workspace directory (including the tool cache) before cascading the workspace's sessions and messages from SQLite. Remote GitHub data is outside this action.

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
