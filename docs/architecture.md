# Perpetual local architecture

## Product boundary

Perpetual is a single-user personal server running on one Linux machine. One long-lived Go process serves the browser UI, calls GitHub, owns SQLite, and runs bounded shell commands for workspace setup. The browser may run on another personal device when a trusted HTTPS proxy or private network protects access. Perpetual deliberately has no Postgres, VM manager, worker fleet, queue, separate agent worker, or provider-plugin runtime; virtual-machine compute is under design as a replacement for the removed Docker sandbox, and a new built-in agent is under design to replace the removed Fireworks-backed agent.

The durable project object is a workspace containing one or more sessions. The Fireworks-backed agent was removed; the chat interface and session storage are kept while a new agent is designed:

```text
Existing or newly created GitHub repository + base/working branch
  -> local clone + SQLite record
  -> deterministic project analysis + dependency initialization
  -> durable session conversations (agent execution being redesigned)
  -> Git status/diff review
  -> commit, push, and draft pull request
  -> user stops or deletes the local workspace
```

The repository, cache, sessions, and SQLite record survive a normal Stop. Stop marks the workspace stopped; Resume returns it to ready without rerunning preparation, so workspace changes remain intact. Delete is a separate confirmed action that removes the local clone, tool cache, workspace record, sessions, and messages; it never deletes the remote GitHub branch or pull request.

## Component ownership

- `cmd/perpetual` owns signal handling and starts the web server.
- `web` owns the React and TypeScript browser interface, its component tests, and the Vite build.
- `internal/database` owns the shared SQLite connection, file permissions, WAL mode, foreign keys, and busy timeout.
- `internal/exec` owns bounded local shell execution for setup and agent commands.
- `internal/workspaceruntime` owns the control-plane/runtime boundary and the local compatibility adapter.
- `internal/webapp` owns HTTP routes, the embedded production bundle, local server startup, and component wiring.
- `internal/workspace` owns the SQLite schema, workspace state, deterministic project analysis, trusted host Git operations, preparation, change inspection, and publish flow.
- `internal/githubapp` owns GitHub user authorization, installed-repository discovery, personal repository creation, branch listing, draft pull requests, and the private credential file.
Infrastructure packages do not depend on `internal/webapp`; the web layer connects consumer-owned interfaces.

The React application calls the existing JSON endpoints and does not own durable state or runtime
lifecycle. Its production build is written to `internal/webapp/dist` and embedded into the Go
binary. Development may use Vite's local proxy, but deployment remains one Go process; there is no
Node.js production server.

## SQLite state

SQLite uses WAL mode, foreign keys, a five-second busy timeout, and one database connection. The schema contains:

- `workspaces`: repository, base and working branches, local path, setup command, lifecycle status, preparation stage/detail, project-root selection, failure, and pull-request identity.
- `sessions`: workspace-scoped conversations with independent titles, run status, failure, and timestamps.
- `messages`: ordered conversation messages, including tool calls and tool results, linked to a session.
- The removed `agent_runs` table was dropped by a schema migration; sessions and stored messages are preserved for the future agent.
- `workspace_environment`: encrypted workspace-scoped values, variable names, setup exposure, and timestamps.
- `workspace_jobs`: durable workspace operations with queued/running/succeeded/failed/canceled state, attempts, lease owner, expiration, and recorded error.
- `workspace_profiles`: project root, languages, runtimes, package managers, lockfiles, resolved commands, manifest fingerprint, clean Git baseline, cache identity, preparation results, and the latest deterministic `EnvironmentSpec`. It never stores secret values.

The Docker environment tables (`environments`, `environment_leases`) and the legacy
`workspaces.sandbox_name` column were removed by a schema migration. The Environments page, its
TypeScript contracts, and client code are kept parked for the planned virtual-machine compute
backend; the JSON environment endpoints no longer exist, so the page reports that environment
management is unavailable.

Workspace lifecycle values describe only the workspace: `creating`, `initializing`, `needs_configuration`, `initialization_failed`, `ready`, and `stopped`. Preparation independently records `pending`, `cloning`, `analyzing`, `installing`, `verifying`, `sealing`, `needs_configuration`, `ready`, or `failed`, plus the stage that failed. Session lifecycle values describe agent work: `idle`, `working`, `review`, `failed`, and `canceled`. Existing workspace conversations are migrated into an `Original session` without losing messages.

Sessions share one workspace clone, branch, cache, and diff. They isolate conversational context and activity history, not filesystem state. Sessions and their stored messages remain durable SQLite state; the message-send and run-cancel endpoints were removed with the agent backend, so the browser composer is currently parked until the new agent defines its execution model. Changes, environment, and publishing stay workspace-scoped.

The browser opens one authenticated same-origin Server-Sent Events stream. The server sends only bounded `session.changed` invalidation notices containing workspace, session, and run IDs; SQLite remains authoritative, and the browser reloads current session/messages after each relevant notice. A capacity-one channel per browser coalesces slow consumers, heartbeats keep compatible proxies from treating an idle stream as dead, and the native `EventSource` client reconnects automatically. No token, prompt, model output, or shell output is placed in the event stream. Disconnecting a browser has no effect on a run.

## Shell execution

The Docker sandbox backend was removed. `internal/exec` now provides a bounded local shell for
dependency setup, change inspection, and publishing commits (and will back the planned agent's shell tool). Commands run through `/bin/sh -c` on the controller machine with a two-minute timeout, 64 KiB command limit, and 32 KiB bounds for each output stream with truncation reporting. Commands execute in the workspace repository directory with a process-group cancel that kills the whole tree on browser Stop or timeout.

Each workspace gets a private environment that never includes host configuration:

- `HOME` points to a private directory under the managed workspace cache;
- tool caches (Go, npm, pip, Cargo, and XDG-compatible tools) live under the managed cache so they survive Stop and Resume;
- `PATH` is inherited so the host Go, Node, and Python toolchains are available;
- workspace environment values are decrypted per command, with setup receiving only values marked for setup;
- configured values are best-effort redacted from captured output before tool results are recorded.

This is an intermediate execution mode while virtual-machine compute is designed; it runs with
host user privileges and is not a hostile-code sandbox. The controller never places GitHub or
workspace secrets in the shell environment, repository URLs, messages, logs, or tests.

Initialization first clones or opens the repository with trusted host Git. A requested new working branch is created only in the local clone. Fixed rules inspect regular metadata files, resolve the repository root or one nested project, and derive Go, Node, and Python setup and verification commands. Multiple nested roots require a later user selection rather than an AI guess. The controller records the current commit and requires a clean Git status before setup.

Each preparation transition is persisted before its work starts, allowing browser polling and process restarts to report truthful progress. Multiple project candidates are stored as non-secret summaries. `POST /api/workspaces/{id}/configure` accepts only a stored candidate, persists the selected root, and restarts deterministic analysis at that root. Failure records both an actionable error and the stage that failed; retry preserves the managed clone, cache, and sessions.

On controller startup, any workspace left in `creating` or `initializing` is atomically moved to `initialization_failed` with its interrupted stage preserved.

Preparation runs dependency setup through the bounded shell contract that the planned agent will reuse. It compares Git status after setup and records any tracked or untracked changes in the project profile so they remain visible for review. `Stop` marks the workspace stopped; `Resume` returns it to ready. Deletion validates that the recorded clone is exactly `<managed-root>/<workspace-id>/repo`, removes the workspace directory including the tool cache, and then deletes the workspace record; foreign-key cascades remove its sessions and messages. Initialization must finish or fail before deletion so clone/setup work cannot recreate files after cleanup.

Environment values use AES-GCM with a random local 256-bit key stored beside the database in a `0600` file. Names and exposure scope are readable metadata; API responses never include values. Mutations are rejected during initialization. Stop preserves values, while workspace deletion removes their rows through the existing foreign-key cascade. This protects against accidental repository, API, and log exposure, not against commands that are intentionally given the values.

## Workspace runtime boundary

The control plane never opens a workspace shell directly. `internal/workspaceruntime`
defines the `Runtime` contract and a `Ref` that identifies a workspace runtime
instance. Workspace lifecycle, preparation, review, publish, and the future
agent all interact with a runtime through this seam instead of constructing a
host shell themselves.

`NewLocal` provides the compatibility implementation: lifecycle calls are
idempotent, `OpenShell` creates the private per-workspace home and returns a
bounded local shell rooted at the workspace repository directory. Cloud-backed
implementations may replace it behind the same contract, which keeps workspace
and webapp code independent of the final execution substrate.

## Environment specification

`internal/workspace` builds a deterministic `EnvironmentSpec` for every prepared
project root. The spec records toolchains, package managers, lockfiles, setup,
verify, build, and test commands, devcontainer services, the metadata source
files, and a source fingerprint. It deliberately never contains secret values.

The analyzer treats `.mise.toml` and `.tool-versions` as primary toolchain
signals, recognizes `devcontainer.json` lifecycle and service features, and
falls back to Go, Node, and Python manifests the way the workspace profile
historically did. The spec is persisted in `workspace_profiles.environment_spec`
so every workspace carries an inspectable description of how its environment is
defined. Reusing and versioning those specs across workspaces is the next stage
of the environment work.

## Durable workspace jobs

Workspace preparation is enqueued as a durable job instead of being started
from an HTTP handler goroutine. `workspace_jobs` records the operation state,
attempts, lease, and error; a single worker loop claims queued jobs and drives
`Initialize` through the workspace runtime. On startup, queued and running jobs
are marked failed as interrupted so the user can retry explicitly, and the
workspace itself is similarly marked for retry.

Browser requests only enqueue work and return accepted. The worker owns
execution, which keeps preparation observable and safe across process
restarts, and gives environment builds and agent runs a shared durable
execution primitive.

## Agent backend (removed)

The Fireworks-backed agent was removed: `internal/agent`, `internal/chat`,
`internal/fireworks`, and `internal/config` (including the `perpetual config` CLI
command) no longer exist, the `agent_runs` table was dropped, and the send/cancel
message endpoints were removed. The startup no longer loads a provider
configuration, so the server runs without a `config.json`.

The browser chat interface, session list, composer, and stored conversation
history are kept as parked client code for the new agent being designed. Opening
a conversation still renders past messages; Send and Stop currently report that
the agent is unavailable because those endpoints are gone.

## GitHub and publish boundary

GitHub OAuth state is kept in an HTTP-only, same-site callback cookie. The user access token is stored in a private local file. Repository selection is checked against repositories returned for the App installations before a workspace is created.

New-project creation uses the authenticated user's GitHub App token and requires the App's `Administration: write` repository permission. The controller asks GitHub to initialize the repository with a README, then uses GitHub's returned full name, clone URL, and default branch when entering the normal workspace pipeline. Private is the product default. The remote operation cannot be atomic with the local SQLite write, so Perpetual reports partial failure and intentionally leaves the GitHub repository recoverable instead of deleting it automatically. GitHub credentials remain controller-only throughout this flow.

Authenticated clone and push use host Git with a short-lived private askpass script. The access token is passed only to that trusted Git child process and removed with the helper; it is never written into the remote URL or exposed to model shell commands. Publishing stages all workspace changes, creates a focused user-supplied commit, pushes the working branch, and asks GitHub to open a draft pull request.

Mutating HTTP endpoints require the non-simple `X-Perpetual-Request: 1` header. The event stream requires the existing GitHub-authenticated personal session. The server binds to `127.0.0.1:8080` by default and supports remote deployment through `PERPETUAL_PUBLIC_URL` (the externally visible URL that derives the GitHub callback URL), optional built-in TLS (`PERPETUAL_TLS_CERT`/`PERPETUAL_TLS_KEY`), and an optional single-user HTTP Basic password gate (`PERPETUAL_ACCESS_PASSWORD`) enforced before every request. Because the GitHub App callback URL cannot point at a wildcard bind, a non-loopback listen address now requires `PERPETUAL_PUBLIC_URL` or `PERPETUAL_GITHUB_CALLBACK_URL`. Remote use should still add HTTPS and access control through a trusted reverse proxy, VPN, or SSH tunnel; Perpetual should not be exposed directly to the public internet without the password gate. Multi-user sessions, webhook validation, installation-token brokerage, queues, and fleet scheduling remain intentionally deferred.
