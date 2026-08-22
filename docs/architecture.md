# Perpetual architecture

## Product boundary

Perpetual is a controller-led server for GitHub-authenticated users. One
long-lived Go process owns authentication, Git, SQLite, workspace state,
scheduling, and review/publish. Workspace setup, change inspection, and agent
command execution run through a bounded shell contract backed by a runtime
provider.

Two runtime providers are part of the product direction:

- **local** — development/compatibility adapter that runs on the controller host.
- **cloud / Lambda MicroVMs** — isolated Firecracker-based microVMs managed by
  AWS Lambda MicroVMs. This is the first production execution substrate.

Both implement the same `Runtime` and `exec.Shell` contracts. A self-managed
Firecracker provider is a future option behind the same seam, not a separate
product path. Perpetual does not require Postgres, an external workflow engine,
Kafka, or a separate worker fleet; durable jobs and leases remain
controller-owned in SQLite.

The durable project object is a workspace containing one or more sessions. The
Fireworks-backed agent was removed; the chat interface and session storage are
kept while the new execution-room agent is designed:

```text
Existing or newly created GitHub repository + base/working branch
  -> workspace SQLite record + prepared working tree
  -> deterministic project analysis + dependency initialization
     (local compatibility runtime now; Lambda MicroVMs provider in build-out)
  -> durable session conversations + execution-room agent runs
  -> Git status/diff review
  -> commit, push, and draft pull request
  -> user stops, resumes, or deletes the workspace and its environment
```

The working tree, caches/snapshots, sessions, and SQLite record survive a
normal Stop. Stop marks the workspace stopped; in cloud mode this maps to
suspending the workspace microVM. Resume returns the workspace to ready without
rerunning preparation, so workspace changes remain intact. Delete is a
separate confirmed action that removes provider-owned compute and local state,
workspace record, sessions, and messages; it never deletes the remote GitHub
branch or pull request.

## Component ownership

- `cmd/perpetual` owns signal handling and starts the web server.
- `web` owns the React and TypeScript browser interface, its component tests, and the Vite build.
- `internal/database` owns the shared SQLite connection, file permissions, WAL mode, foreign keys, and busy timeout.
- `internal/accounts` owns GitHub-linked users, server-side login sessions, and encrypted per-user GitHub credentials.
- `internal/exec` owns the bounded shell execution contract and its local implementation.
- `internal/workspaceruntime` owns the control-plane/runtime boundary, the local adapter, and the cloud provider implementation.
- `internal/webapp` owns HTTP routes, the embedded production bundle, control-plane wiring, and component composition.
- `internal/workspace` owns the SQLite schema, workspace state, deterministic project analysis, trusted host Git operations, preparation, change inspection, execution-room state, and publish flow.
- `internal/githubapp` owns GitHub user authorization, installed-repository discovery, personal repository creation, branch listing, draft pull requests, and the private credential store.

Planned packages for the cloud build-out:

- `internal/environments` owns microVM image/instance management and the
  provider-specific AWS SDK calls.
- `internal/execution` or `internal/agent` owns the execution-room loop, durable
  step journal, target/model invocation, and single `shell(command)` tool.

Infrastructure packages do not depend on `internal/webapp`; the web layer
connects consumer-owned interfaces.

The React application calls JSON endpoints and does not own durable state or
runtime lifecycle. Its production build is written to `internal/webapp/dist`
and embedded into the Go binary. Development may use Vite's local proxy, but
deployment remains one Go process; there is no Node.js production server.

## SQLite state

SQLite uses WAL mode, foreign keys, a five-second busy timeout, and one database
connection. The schema contains:

- `workspaces`: user ownership, runtime provider/ref/state, repository, base and
  working branches, local path, setup command, lifecycle status, preparation
  stage/detail, project-root selection, failure, environment binding, and
  pull-request identity.
- `users`: internal ID, unique GitHub ID, login, display name, avatar URL, and timestamps.
- `auth_sessions`: user-linked sessions storing only a SHA-256 token hash, expiry, revocation state, and timestamps.
- `github_credentials`: user-linked GitHub access tokens encrypted with AES-GCM and never returned by APIs.
- `sessions`: workspace-scoped conversations with independent titles, run status, failure, and timestamps.
- `messages`: ordered conversation messages, including tool calls and tool results, linked to a session.
- `workspace_environment`: encrypted workspace-scoped values, variable names, setup exposure, and timestamps.
- `workspace_jobs`: durable workspace operations with owner, queued/running/succeeded/failed/canceled state, attempts, lease owner, expiration, and recorded error.
- `project_environments`: stable environment identity per user + repository + project root.
- `environment_versions`: source-fingerprinted environment builds with pending/ready/failed state, the environment spec, artifact/cache references, and build error.
- `workspace_profiles`: project root, languages, runtimes, package managers, lockfiles, resolved commands, manifest fingerprint, clean Git baseline, cache identity, preparation results, and the latest deterministic `EnvironmentSpec`. It never stores secret values.

The old Fireworks `agent_runs` table was dropped by a schema migration.
Execution-room work is planned to reintroduce durable run state as:

- `agent_runs`: run ID, session/workspace/user ownership, state, step cursor,
  deadline, lease, heartbeat, prompt/model version, result, and error.
- `agent_run_steps`: idempotent step journal keyed by `(run_id, step_key)` with
  input JSON and output JSON saved after success.
- `agent_work_memory`: compact run-scoped notes/todos used for long-task context.

The Docker environment tables (`environments`, `environment_leases`) and the
legacy `workspaces.sandbox_name` column were removed by a schema migration. The
Environments page, its TypeScript contracts, and client code remain parked for
the Lambda MicroVMs backend. The current environment JSON endpoints are absent,
so the page reports that environment management is unavailable until the cloud
provider is wired.

Workspace lifecycle values describe only the workspace: `creating`,
`initializing`, `needs_configuration`, `initialization_failed`, `ready`, and
`stopped`. Preparation independently records `pending`, `cloning`, `analyzing`,
`installing`, `verifying`, `sealing`, `needs_configuration`, `ready`, or
`failed`, plus the stage that failed. Session/run lifecycle values describe
execution-room work: `idle`, `queued`, `working`, `waiting_user`, `review`,
`failed`, and `canceled`. Existing workspace conversations are migrated into an
`Original session` without losing messages.

Sessions share one workspace working tree, branch, cache/snapshot, and diff.
They isolate conversational context and activity history, not filesystem state.
Sessions and their stored messages remain durable SQLite state; the message-send
and run-cancel endpoints were removed with the old agent backend, so the
browser composer is currently parked until the execution-room agent defines its
model provider and loop.

The browser opens one authenticated same-origin Server-Sent Events stream. The
server sends only bounded `session.changed` invalidation notices containing
workspace, session, and run IDs; SQLite remains authoritative, and the browser
reloads current session/messages after each relevant notice. A capacity-one
channel per browser coalesces slow consumers, heartbeats keep compatible proxies
from treating an idle stream as dead, and the native `EventSource` client
reconnects automatically. No token, prompt, model output, or shell output is
placed in the event stream. Disconnecting a browser has no effect on a run.

## Authentication and user scoping

GitHub OAuth remains the only sign-in method. The web layer uses the existing
GitHub consent flow to prove identity, then `internal/accounts` persists a
GitHub-linked user and creates a server-side `perpetual_session` cookie. The
cookie value is a random opaque token; SQLite stores only its SHA-256 hash.
Sessions have a bounded lifetime, can be revoked on logout, are checked by web
middleware before user-scoped workspace routes run, and are periodically removed
after expiry by a background cleanup loop.

Each workspace records a `user_id`. The web API lists and gets workspaces with
`WHERE user_id = ?`, and workspace actions verify ownership before invoking the
workspace service. `workspace_jobs` records the workspace owner, and
`project_environments` is unique per `user_id`, repository, and project root so
environment versions never leak across users. Environment lookups join the
environment owner before returning a version. Rows that predate ownership keep
an empty `user_id`. On the very first GitHub login against a pre-tenancy
database, the web layer claims those rows for that user: workspaces directly,
jobs through their workspace, and environments through matching workspace
profiles. The claim is idempotent and only fires when no user existed yet, so
hosted databases never auto-claim.

The execution-room model extends the same scoping rule: every run, step, and
work-memory row is owned by a user, workspace, and session. A full multi-tenant
SaaS control plane is still out of scope, but the data model is user-scoped so
multiple GitHub accounts can coexist without leaking environment or run state.

The OAuth callback stores the GitHub access token in `github_credentials` for
that user, encrypted with AES-GCM under a local `github.key` file. The browser
receives only the opaque `perpetual_session` cookie. Web handlers load the
current user's encrypted token before calling GitHub. `internal/workspace`
depends on a small `GitHubTokenProvider` seam rather than the account store;
`internal/webapp` injects `accounts.Store` so background prepare, build, and
agent jobs use the token belonging to the workspace owner.

## Shell execution

`internal/exec` owns the bounded shell contract used by setup, change
inspection, publishing, and the planned agent:

- `/bin/sh -c` execution;
- two-minute command timeout;
- 64 KiB command limit;
- 32 KiB per-output-stream bound with truncation reporting;
- process-group cancellation on Stop or deadline;
- best-effort redaction of configured values from captured output.

The local runtime executes this contract on the controller host and is a
development/compatibility adapter. It is not a hostile-code sandbox. The Lambda
MicroVMs runtime executes the same contract inside an isolated microVM by
sending a serialized shell request to an in-VM `vmagent` over the authenticated
data-plane endpoint.

Each workspace gets a private execution environment:

- local mode points `HOME` at a private directory under the managed workspace cache;
- local tool caches (Go, npm, pip, Cargo, and XDG-compatible tools) live under the managed cache so they survive Stop and Resume;
- cloud mode carries that state in the microVM snapshot/working tree rather than a host directory;
- `PATH` is inherited in local mode; the microVM image provides the toolchain in cloud mode;
- workspace environment values are decrypted per command, with setup receiving only values marked for setup;
- configured values are best-effort redacted from captured output before tool results are recorded;
- the controller never places GitHub or AWS control-plane credentials in the shell environment.

Initialization first clones or opens the repository with trusted host Git. A
requested new working branch is created only in the working tree. Fixed rules
inspect regular metadata files, resolve the repository root or one nested
project, and derive Go, Node, and Python setup and verification commands.
Multiple nested roots require a later user selection rather than an AI guess.
The controller records the current commit and requires a clean Git status before
setup.

Each preparation transition is persisted before its work starts, allowing
browser polling and process restarts to report truthful progress. Multiple
project candidates are stored as non-secret summaries.
`POST /api/workspaces/{id}/configure` accepts only a stored candidate, persists
the selected root, and restarts deterministic analysis at that root. Failure
records both an actionable error and the stage that failed; retry preserves the
managed working tree, caches/snapshots, and sessions.

On controller startup, any workspace left in `creating` or `initializing` is
atomically moved to `initialization_failed` with its interrupted stage preserved.

Preparation runs dependency setup through the bounded shell contract that the
execution-room agent will reuse. It compares Git status after setup and records
any tracked or untracked changes in the project profile so they remain visible
for review. `Stop` marks the workspace stopped; `Resume` returns it to ready.
Deletion validates local paths before removal and terminates/disposes
provider-owned compute before cascading workspace records. Initialization must
finish or fail before deletion so setup work cannot recreate state after
cleanup.

Environment values use AES-GCM with a random local 256-bit key stored beside the
database in a `0600` file. Names and exposure scope are readable metadata; API
responses never include values. Mutations are rejected during initialization.
Stop preserves values, while workspace deletion removes their rows through the
existing foreign-key cascade. In cloud mode values are sent only to the
authenticated microVM endpoint at shell-execution time and purged on suspend.
This protects against accidental repository, API, console, and log exposure, not
against commands that are intentionally given the values.

## Workspace runtime boundary

The control plane never opens a workspace shell directly. `internal/workspaceruntime`
defines the `Runtime` contract and a `Ref` that identifies a workspace runtime
instance. Workspace lifecycle, preparation, review, publish, and execution rooms
all interact with a runtime through this seam.

The target lifecycle mapping is:

```text
Start   -> RunMicrovm or equivalent provider action
Stop    -> SuspendMicrovm for resumable state; local adapter stays idempotent
Resume  -> ResumeMicrovm for microVMs; local adapter stays idempotent
Destroy -> TerminateMicrovm, then delete provider-specific state
OpenShell -> local exec.Shell or HTTP-backed microVM exec.Shell
```

`NewLocal` remains the compatibility implementation: lifecycle calls are
idempotent, `OpenShell` creates the private per-workspace home and returns a
bounded local shell rooted at the workspace repository directory. The Lambda
MicroVMs provider is under implementation behind the same contract, which keeps
workspace and webapp code independent of the execution substrate.

Runtime selection remains explicit via `PERPETUAL_RUNTIME` (default `local`) or
the `-runtime` flag. The existing `cloud` selector currently returns a stub that
fails with a clear "not implemented" error so misconfiguration never silently
falls back to the host. Every workspace records `runtime_provider`,
`runtime_ref`, `runtime_state` (`not_created`, `creating`, `running`,
`stopped`, `destroying`, `failed`), and `runtime_updated_at` in SQLite, and the
controller persists state transitions before and after runtime lifecycle calls.

The provider-specific cloud details live in `cloud-architecture.md`.

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
defined. In cloud mode the same spec is the input for producing a deterministic
microVM image.

## Environment versions

Each prepared workspace finds or creates a stable `project_environments` row and
binds to an `environment_versions` row. The version stores the `EnvironmentSpec`
JSON and its source fingerprint, a pending/ready/failed state, and an
artifact/cache reference. Workspace `environment_version_id` records the
binding.

Preparation first looks for an existing ready version with the same fingerprint.
If found, the workspace binds to it. If not, a pending version is created, the
workspace binds to it, setup runs, and the version is marked ready on success or
failed on error. This keeps environment identity separate from branches and
sessions. For local runtime an environment version currently maps to setup plus
a cache/snapshot reference. For cloud runtime a ready version maps to a built
Lambda MicroVM image, represented by image ARN and image version in the
provider-specific artifact reference.

## Environment snapshot reuse

Successful `build_environment` jobs capture the project's ignored and untracked
setup outputs (for example `node_modules`, `.venv`, `vendor`, build directories)
into a managed `environment-snapshots/{version-id}` directory. The snapshot
manifest is stored on the environment version.

When `prepare_workspace` finds a ready version with a usable snapshot, it
restores the snapshot into the workspace and skips dependency setup, recording
`SetupResult = "restored"`. If no snapshot exists or restoration fails, it falls
back to running setup normally. For the local runtime, snapshots live only under
the managed data root, exclude `.git` and symlinks, and are bounded to one
gibibyte. For cloud runtime, the equivalent optimization is the Lambda MicroVM
snapshot captured at image build time.

## Durable workspace jobs

Workspace preparation is enqueued as a durable job instead of being started from
an HTTP handler goroutine. `workspace_jobs` records the operation state,
attempts, lease, and error; a worker loop claims queued jobs and drives
`Initialize` through the workspace runtime. On startup, queued and running jobs
are marked failed as interrupted so the user can retry explicitly, and the
workspace itself is similarly marked for retry.

Browser requests only enqueue work and return accepted. The worker owns
execution, which keeps preparation observable and safe across process restarts,
and gives environment builds and execution-room runs a shared durable execution
primitive.

`prepare_workspace` runs workspace initialization, and `build_environment`
rebuilds or verifies a workspace's bound environment version through the same
runtime seam. `POST /api/workspaces/{id}/environment/rebuild` enqueues a
`build_environment` job for a pending or failed version and returns accepted;
the worker runs dependency setup, saves the profile result, and marks the
version ready or failed.

When workspace preparation creates a new environment version, it moves the
workspace to `waiting_environment` and enqueues `build_environment` instead of
running setup inline. The build job then installs dependencies, marks the
version ready or produces a MicroVM image, and finalizes the waiting workspace.
Reused ready versions are still materialized inline by `prepare_workspace` for
the local runtime.

## Execution rooms and durable runs

Execution rooms are the planned replacement for the removed inline agent
backend. One execution room corresponds to one session in one workspace and
follows the durable state machine:

```text
queued -> running -> waiting_user -> completed / failed / canceled
```

Each loop iteration is an atomic step: assemble bounded context, call the model,
parse a tool request, execute `shell(command)` through the selected runtime,
persist the tool result, checkpoint the step, then continue or stop.

Durability rules:

- runs and steps live in SQLite and are user/workspace scoped;
- a step is written only after success and reused by idempotent key on resume;
- a running room updates a heartbeat and lease;
- runs have step timeouts, a total deadline, max steps, and token/output bounds;
- Stop cancels the run and the underlying shell process group;
- progress is surfaced through the existing SSE invalidation stream without
  placing prompt or secret values in events.

The full loop, concurrency, and context design is in
`plans/execution-loops-and-context.md`.

## Agent backend (removed)

The Fireworks-backed agent was removed: `internal/agent`, `internal/chat`,
`internal/fireworks`, and `internal/config` (including the `perpetual config` CLI
command) no longer exist, the old `agent_runs` table was dropped, and the
send/cancel message endpoints were removed. The startup no longer loads a
Fireworks provider configuration.

The browser chat interface, session list, composer, and stored conversation
history are kept as parked client code for the execution-room agent. Opening a
conversation still renders past messages; Send and Stop currently report that
the agent is unavailable because the loop/model provider is not wired yet.

## GitHub and publish boundary

GitHub OAuth state is kept in an HTTP-only, same-site callback cookie. The user
access token is stored encrypted in `github_credentials`. Repository selection
is checked against repositories returned for the App installations before a
workspace is created.

New-project creation uses the authenticated user's GitHub App token and requires
the App's `Administration: write` repository permission. The controller asks
GitHub to initialize the repository with a README, then uses GitHub's returned
full name, clone URL, and default branch when entering the normal workspace
pipeline. Private is the product default. The remote operation cannot be atomic
with the local SQLite write, so Perpetual reports partial failure and
intentionally leaves the GitHub repository recoverable instead of deleting it
automatically. GitHub credentials remain controller-only throughout this flow.

Authenticated clone and push use host Git with a short-lived private askpass
script. The access token is passed only to that trusted Git child process and
removed with the helper; it is never written into the remote URL or exposed to
agent shell commands. Publishing stages working-tree changes, creates a focused
user-supplied commit, pushes the working branch, and asks GitHub to open a draft
pull request. In cloud mode the controller first syncs the working tree back
from the microVM to its trusted review path.

Mutating HTTP endpoints require the non-simple `X-Perpetual-Request: 1` header.
The event stream requires the existing GitHub-authenticated personal session.
The server binds to `127.0.0.1:8080` by default and supports remote deployment
through `PERPETUAL_PUBLIC_URL` (the externally visible URL that derives the
GitHub callback URL), optional built-in TLS
(`PERPETUAL_TLS_CERT`/`PERPETUAL_TLS_KEY`), and an optional HTTP Basic password
gate (`PERPETUAL_ACCESS_PASSWORD`) enforced before every request. Because the
GitHub App callback URL cannot point at a wildcard bind, a non-loopback listen
address now requires `PERPETUAL_PUBLIC_URL` or
`PERPETUAL_GITHUB_CALLBACK_URL`. Remote use should still add HTTPS and access
control through a trusted reverse proxy, VPN, or SSH tunnel. The cloud runtime
does not make the control plane itself a Kubernetes-style multi-tenant SaaS;
webhook validation, installation-token brokerage, and fleet scheduling remain
intentionally deferred.
