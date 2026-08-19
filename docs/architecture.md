# Perpetual local architecture

## Product boundary

Perpetual is a single-user personal server running on one Linux machine. One long-lived Go process serves the browser UI, calls GitHub and Fireworks, owns SQLite, and controls local Docker containers. The browser may run on another personal device when a trusted HTTPS proxy or private network protects access. Perpetual deliberately has no Postgres, VM manager, worker fleet, queue, separate agent worker, or provider-plugin runtime.

The durable project object is a workspace containing one or more sessions. Every run uses the built-in Perpetual agent and the configured Fireworks model:

```text
Existing or newly created GitHub repository + base/working branch
  -> local clone + SQLite record
  -> exclusive lease on one reusable environment
  -> disposable, generation-identified Docker runtime
  -> deterministic project analysis + dependency initialization
  -> separate durable session chats and explicit agent runs
  -> Git status/diff review
  -> commit, push, and draft pull request
  -> user stops workspace and releases environment capacity
```

The repository, cache, sessions, and SQLite record survive a normal Stop. Stop destroys the exact leased runtime and releases the environment. Resume acquires available capacity and creates a new writable runtime without rerunning preparation, so workspace changes remain intact. A ready workspace restores and verifies its active leased runtime after a controller restart. Delete is a separate confirmed action that first proves no live or uncertain runtime remains, then removes the local clone, workspace record, sessions, and messages; it never deletes the remote GitHub branch or pull request.

## Component ownership

- `cmd/perpetual` owns signal handling and selects the web server or the small `config` command.
- `web` owns the React and TypeScript browser interface, its component tests, and the Vite build.
- `internal/database` owns the shared SQLite connection, file permissions, WAL mode, foreign keys, and busy timeout.
- `internal/environment` owns reusable compute definitions, exclusive generation-checked workspace leases, and runtime lifecycle coordination.
- `internal/webapp` owns HTTP routes, the embedded production bundle, local server startup, and component wiring.
- `internal/workspace` owns the SQLite schema, workspace state, deterministic project analysis, trusted host Git operations, preparation, change inspection, and publish flow.
- `internal/sandbox` owns disposable Docker-runtime creation, restoration and removal, the Docker environment driver, and bounded shell execution.
- `internal/githubapp` owns GitHub user authorization, installed-repository discovery, personal repository creation, branch listing, draft pull requests, and the private credential file.
- `internal/chat` binds each durable session conversation to the agent loop and permits only one active run per workspace.
- `internal/agent` owns the built-in prompt, shared messages, and the sequential loop with a hard 20-decision ceiling.
- `internal/fireworks` owns the Fireworks request format and implements the shared agent-provider contract.
- `internal/config` owns the private Fireworks key and model configuration and the terminal setup command.

Infrastructure packages do not depend on `internal/webapp`; the web layer connects consumer-owned interfaces.

The React application calls the existing JSON endpoints and does not own durable state or runtime
lifecycle. Its production build is written to `internal/webapp/dist` and embedded into the Go
binary. Development may use Vite's local proxy, but deployment remains one Go process; there is no
Node.js production server.

## SQLite state

SQLite uses WAL mode, foreign keys, a five-second busy timeout, and one database connection. The schema contains:

- `workspaces`: repository, base and working branches, local path, setup command, lifecycle status, preparation stage/detail, project-root selection, failure, and pull-request identity.
- `sessions`: workspace-scoped conversations with independent titles, run status, failure, and timestamps.
- `messages`: ordered full agent messages, including tool calls and tool results, linked to a session.
- `agent_runs`: durable accepted work with exact workspace/session ownership, lifecycle, failure, and timestamps.
- `workspace_environment`: encrypted workspace-scoped values, variable names, setup exposure, and timestamps.
- `workspace_profiles`: project root, languages, runtimes, package managers, lockfiles, resolved commands, manifest fingerprint, clean Git baseline, cache identity, and preparation results. It never stores secret values.
- `environments`: reusable runtime configuration, resolved image identity, resource policy, provisioning health, and lease generation.
- `environment_leases`: durable acquiring, active, releasing, released, or failed assignments with unique active environment and workspace constraints.

On first startup the controller registers one ready Local Docker environment using the immutable
ID resolved from the configured sandbox image. The Environments page and guarded JSON endpoints
list capacity, create additional local Docker configurations, retry failed image resolution, and
delete only unoccupied environments. Configuration is fixed at creation so resource policy cannot
change underneath an active lease. Workspace creation, preparation, shell access, Stop, Resume,
recovery, and deletion all use `internal/environment.RuntimeService`; no
workspace-derived container name is a lifecycle authority.

Environment selection is deliberately automatic. The controller allocates any ready capacity in
one SQLite transaction rather than asking the user to coordinate leases. The workspace header
resolves its active lease to a human-readable environment, and the Environments page resolves an
occupied lease back to its workspace. When all environments are occupied, acquisition fails
without overcommitting; releasing one workspace makes that environment available to another with
a new lease generation.

Workspace lifecycle values describe only the environment: `creating`, `initializing`, `needs_configuration`, `initialization_failed`, `ready`, and `stopped`. Preparation independently records `pending`, `cloning`, `analyzing`, `installing`, `verifying`, `sealing`, `needs_configuration`, `ready`, or `failed`, plus the stage that failed. Session lifecycle values describe agent work: `idle`, `working`, `review`, `failed`, and `canceled`. Existing workspace conversations are migrated into an `Original session` without losing messages.

Sessions share one workspace clone, branch, cache, leased environment, runtime, and diff. They isolate conversational context and activity history, not filesystem state. Before accepting a run, one SQLite transaction creates its `agent_runs` row, records the user message, marks the session working, and enforces one active run per workspace. Execution is then owned by the Go process context rather than the initiating HTTP request, so closing or reconnecting the browser does not cancel it. The exact run ID is required for Stop, so a stale or cross-session cancellation cannot stop later work. Changes, environment, and publishing are workspace-scoped, while conversation, run cancellation, and internal activity are session-scoped.

The browser opens one authenticated same-origin Server-Sent Events stream. The server sends only bounded `session.changed` invalidation notices containing workspace, session, and run IDs; SQLite remains authoritative, and the browser reloads current session/messages after each relevant notice. A capacity-one channel per browser coalesces slow consumers, heartbeats keep compatible proxies from treating an idle stream as dead, and the native `EventSource` client reconnects automatically. No token, prompt, model output, or shell output is placed in the event stream. Disconnecting a browser has no effect on a run.

## Sandbox lifecycle

`internal/environment.RuntimeService` now owns the exact compute transition. Start first acquires
one durable generation-checked lease, asks the selected driver to create and verify a runtime, and
activates the lease only with the returned Docker identity. Stop first marks the active lease as
releasing, destroys the exact verified runtime, and only then releases the environment for another
workspace. Creation or destruction failure moves the lease to failed and quarantines the
environment instead of making uncertain compute available again.

The Docker driver names each disposable runtime from its environment and lease generation and
labels it with the exact environment, workspace, lease, and generation identities. Before returning
or deleting a runtime it inspects effective Docker metadata and verifies the resolved image ID,
read-only root, non-root user, CPU, memory, PID, network, privilege, restart, tmpfs, workspace, and
cache policies. Extra persistent mounts, mismatched labels, or stale generations are rejected. The
driver can safely reuse a matching stopped container after a controller interruption, while a
container belonging to another lease is never started or removed.

Environment provisioning is synchronous and controller-owned. The browser supplies only a name,
local image reference, bounded CPU/memory/PID values, and either outbound or disabled networking.
The controller resolves the local image reference to a validated immutable Docker identity before
marking capacity available. Resolution failure persists a failed environment for explicit Repair;
there is no background provisioner, arbitrary Docker argument field, remote host, or agent-facing
environment lifecycle tool. A failed runtime lease is not treated as an image-provisioning failure:
its environment remains visibly quarantined and cannot be repaired or deleted through environment
management until deletion of the owning failed workspace proves runtime cleanup and removes the
failed lease.

Initialization first clones or opens the repository with trusted host Git. A requested new working branch is created only in the local clone. Fixed rules inspect regular metadata files, resolve the repository root or one nested project, and derive Go, Node, and Python setup and verification commands. Multiple nested roots require a later user selection rather than an AI guess. The controller records the current commit and requires a clean Git status before setup.

Each preparation transition is persisted before its work starts, allowing browser polling and process restarts to report truthful progress. Multiple project candidates are stored as non-secret summaries. `POST /api/workspaces/{id}/configure` accepts only a stored candidate, persists the selected root, and restarts deterministic analysis at that root. Failure records both an actionable error and the stage that failed; retry preserves the managed clone, cache, environment metadata, and sessions.

On controller startup, any workspace left in `creating` or `initializing` is atomically moved to `initialization_failed` with its interrupted stage preserved. The controller releases its acquiring or active runtime before accepting requests, preventing a detached setup command from retaining the workspace. Accepted or running agent work is marked `interrupted`, and its session is marked failed; Perpetual does not pretend a process restart can resume an in-memory provider or shell call. Ready workspaces restore and verify the runtime recorded by their active lease. If capacity or runtime restoration is unavailable, the workspace becomes stopped with an actionable message. Stopped, failed, and configuration-waiting workspaces do not acquire capacity.

Preparation acquires a ready environment, creates a writable generation-identified runtime, and runs dependency setup through the same bounded shell contract later used by the agent. It compares Git status after setup and records any tracked or untracked changes in the project profile so they remain visible for review. The active lease and runtime span chat turns; `Stop` destroys that exact runtime before releasing capacity.

Deletion waits for canceled agent work to finish, validates that the recorded clone is exactly `<managed-root>/<workspace-id>/repo`, and asks the runtime service to release an active lease or clean up the exact runtime recorded by a failed lease. An uncertain runtime blocks deletion and keeps its environment quarantined. Only after compute cleanup succeeds does Perpetual remove the workspace directory and SQLite record. Foreign-key cascades remove its sessions and messages. Initialization must finish or fail before deletion so clone/setup work cannot recreate files after cleanup.

The container boundary includes:

- non-root image user;
- read-only container root;
- all Linux capabilities dropped and `no-new-privileges`;
- 256 PID, 2 GiB memory, and 2 CPU limits;
- private temporary and home tmpfs mounts;
- one verified read-write repository bind mount;
- writable private `/tmp`, `/home/perpetual`, and `/run/perpetual` tmpfs mounts;
- a workspace-owned `/cache` bind outside the repository, preserved across container recreation;
- no Docker socket, host home, GitHub token, or model-provider key.

Commands run through `docker exec -i` and a fixed launcher with a two-minute timeout, 64 KiB command limit, 32 KiB bounds for each output stream, truncation reporting, and controller cancellation. The controller supplies fixed cache variables for Go, npm, pip, Cargo, and XDG-compatible tools. It decrypts the current workspace values for each command and sends shell-quoted exports over standard input; they are never Docker command arguments or permanent container environment. Setup receives only values explicitly marked for setup. Exact configured values are redacted from captured output before tool results are recorded, though transformed values cannot be reliably recognized. Network isolation is deferred because initial dependency installation requires network access.

Environment values use AES-GCM with a random local 256-bit key stored beside the database in a `0600` file. Names and exposure scope are readable metadata; API responses never include values. Mutations are rejected during initialization or an active agent run. Stop preserves values, while workspace deletion removes their rows through the existing foreign-key cascade. This protects against accidental repository, API, log, and Docker-metadata exposure, not against commands that are intentionally given the values.

## Agent execution

Perpetual has one built-in coding agent and one provider: Fireworks. The private configuration file contains the Fireworks API key and model selected through the terminal `perpetual config` command. The browser has no provider configuration or model-discovery surface.

At the start of a run, `internal/chat` loads the session history and current workspace facts, then calls the configured Fireworks model with the built-in prompt. Conversations remain session-specific while the repository and its uncommitted changes remain workspace-wide.

The model receives exactly one function:

```json
{"name":"shell","arguments":{"command":"go test ./..."}}
```

There are no file, GitHub, service, lifecycle, or database tools exposed to the model. The web controller owns workspace creation, initialization, stopping, Git credentials, commits, pushes, and pull requests.

The composer has one Send action and a Stop action while that session owns the active run. Stop cancels the shared Fireworks and shell context, preserves messages and activity already recorded, and moves the session to `canceled` instead of presenting a user decision as a failure. Discussion, planning, and review requests do not grant permission to modify files; the user must state an explicit implementation request. Every run receives the resolved project root, language/runtime/package-manager facts, baseline commit, preparation result, and detected verification commands. Publishing and workspace lifecycle remain unavailable through the model-facing shell.

## GitHub and publish boundary

GitHub OAuth state is kept in an HTTP-only, same-site callback cookie. The user access token is stored in a private local file. Repository selection is checked against repositories returned for the App installations before a workspace is created.

New-project creation uses the authenticated user's GitHub App token and requires the App's `Administration: write` repository permission. The controller asks GitHub to initialize the repository with a README, then uses GitHub's returned full name, clone URL, and default branch when entering the normal workspace pipeline. Private is the product default. The remote operation cannot be atomic with the local SQLite write, so Perpetual reports partial failure and intentionally leaves the GitHub repository recoverable instead of deleting it automatically. GitHub credentials remain controller-only throughout this flow.

Authenticated clone and push use host Git with a short-lived private askpass script. The access token is passed only to that trusted Git child process and removed with the helper; it is never written into the remote URL or exposed to the model sandbox. Publishing stages all workspace changes, creates a focused user-supplied commit, pushes the working branch, and asks GitHub to open a draft pull request.

Mutating HTTP endpoints require the non-simple `X-Perpetual-Request: 1` header. The event stream requires the existing GitHub-authenticated personal session. The server binds to `127.0.0.1:8080` by default. Remote use must add HTTPS and access control through a trusted reverse proxy, VPN, or SSH tunnel; Perpetual should not be exposed directly to the public internet. Multi-user sessions, webhook validation, installation-token brokerage, queues, and fleet scheduling remain intentionally deferred.
