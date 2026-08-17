# Ayati local architecture

## Product boundary

Ayati is a single-user application running on one Linux machine. One Go process serves the browser UI, calls GitHub and configured model providers, owns SQLite, and controls local Docker containers. It deliberately has no Postgres, VM manager, worker fleet, queue, background agent service, or executable provider-plugin runtime.

The durable project object is a workspace containing one or more sessions. Reusable agent definitions are global configuration and are not owned by a workspace:

```text
Existing or newly created GitHub repository + base/working branch
  -> local clone + SQLite record
  -> one named Docker sandbox
  -> deterministic project analysis + dependency initialization
  -> separate durable session chats and explicit agent runs
  -> Git status/diff review
  -> commit, push, and draft pull request
  -> user stops sandbox
```

The repository and SQLite record survive a normal Stop. Resume recreates the named sandbox at the stored authority without rerunning preparation, so Develop changes remain intact. A ready workspace restores its named container if the controller process or container was restarted. Delete is a separate confirmed action that removes the managed container, local clone, workspace record, sessions, and messages; it never deletes the remote GitHub branch or pull request.

## Component ownership

- `cmd/ayati` owns signal handling and selects the web server or the small `config` command.
- `web` owns the React and TypeScript browser interface, its component tests, and the Vite build.
- `internal/webapp` owns HTTP routes, the embedded production bundle, local server startup, and component wiring.
- `internal/workspace` owns the SQLite schema, workspace state, deterministic project analysis, trusted host Git operations, preparation, change inspection, and publish flow.
- `internal/sandbox` owns persistent Docker-container creation, restoration, removal, and bounded shell execution.
- `internal/githubapp` owns GitHub user authorization, installed-repository discovery, personal repository creation, branch listing, draft pull requests, and the private credential file.
- `internal/chat` binds each durable session conversation to the agent loop and permits only one active run per workspace.
- `internal/agent` owns agent and skill definitions, prompt composition, shared messages, and the sequential loop with a hard 20-decision ceiling.
- `internal/provider` owns validated provider definitions, registration, discovery, and runtime resolution.
- `internal/fireworks` owns the Fireworks request format and implements the shared agent-provider contract.
- `internal/openaichat` owns the shared OpenAI-compatible Chat Completions format and connection verification.
- `internal/config` owns versioned private provider configuration, legacy Fireworks migration, and the terminal setup command.

Infrastructure packages do not depend on `internal/webapp`; the web layer connects consumer-owned interfaces.

The React application calls the existing JSON endpoints and does not own durable state or runtime
lifecycle. Its production build is written to `internal/webapp/dist` and embedded into the Go
binary. Development may use Vite's local proxy, but deployment remains one Go process; there is no
Node.js production server.

## SQLite state

SQLite uses WAL mode, foreign keys, a five-second busy timeout, and one database connection. The schema contains:

- `workspaces`: repository, branch, authority, effective mount mode, local path, sandbox name, setup command, lifecycle status, preparation stage/detail, project-root selection, failure, and pull-request identity.
- `sessions`: workspace-scoped conversations with independent titles, run status, failure, and timestamps.
- `messages`: ordered full agent messages, including tool calls and tool results, linked to a session.
- `agents`: global built-in and custom execution profiles containing identity, provider and model, step budget, shell capability, instructions, revision, and archive state.
- `skills`: global reusable Markdown guidance with revision and archive state.
- `agent_skills`: ordered custom-agent skill attachments.
- `application_settings`: the single global default-agent reference.
- `workspace_environment`: encrypted workspace-scoped values, variable names, setup exposure, and timestamps.
- `workspace_profiles`: project root, languages, runtimes, package managers, lockfiles, resolved commands, manifest fingerprint, clean Git baseline, cache identity, and preparation results. It never stores secret values.

Workspace lifecycle values describe only the environment: `creating`, `initializing`, `needs_configuration`, `initialization_failed`, `ready`, and `stopped`. Preparation independently records `pending`, `cloning`, `analyzing`, `installing`, `verifying`, `sealing`, `needs_configuration`, `ready`, or `failed`, plus the stage that failed. Session lifecycle values describe agent work: `idle`, `working`, `review`, `failed`, and `canceled`. Existing workspace conversations are migrated into an `Original session` without losing messages.

Sessions share one workspace clone, branch, sandbox, environment, and diff. They isolate conversational context and activity history, not filesystem state. Each session stores its selected global agent; new sessions copy the current default while existing sessions keep their selection. The controller rejects a second agent run while another session in the same workspace is active. An active run is bound to its session so a stale or cross-session cancellation cannot stop later work. Changes, environment, and publishing are therefore workspace-scoped, while conversation, agent selection, run cancellation, and internal activity are session-scoped.

## Sandbox lifecycle

Initialization first clones or opens the repository with trusted host Git. A requested new working branch is created only in the local clone. Fixed rules inspect regular metadata files, resolve the repository root or one nested project, and derive Go, Node, and Python setup and verification commands. Multiple nested roots require a later user selection rather than an AI guess. The controller records the current commit and requires a clean Git status before setup.

Each preparation transition is persisted before its work starts, allowing browser polling and process restarts to report truthful progress. Multiple project candidates are stored as non-secret summaries. `POST /api/workspaces/{id}/configure` accepts only a stored candidate, persists the selected root, and restarts deterministic analysis at that root. Failure records both an actionable error and the stage that failed; retry preserves the managed clone, cache, environment metadata, and sessions.

On controller startup, any workspace left in `creating` or `initializing` is atomically moved to `initialization_failed` with its interrupted stage preserved. The controller removes its named preparation sandbox before accepting requests, preventing a detached setup command from retaining a writable Explore mount. Active sessions interrupted by restart are similarly marked failed. Ready, stopped, failed, and configuration-waiting workspaces remain unchanged.

Ayati then creates `ayati-workspace-<id>` with a writable preparation mount and runs dependency setup through the same bounded shell contract later used by the agent. It compares Git status after setup. Explore fails preparation if tracked or non-ignored files changed; Develop records those changes and continues. Before Explore becomes ready, the controller removes that writable container, recreates `/workspace` read-only, verifies Docker's effective mount metadata, and records it. Develop remains read-write. The ready container stays alive across chat turns and controller restarts; `Stop` removes only that validated Ayati container name.

Authority changes are synchronous controller operations guarded by the chat service's per-workspace run lock. Explore to Develop validates and creates a local working branch when needed, removes the old container, recreates it read-write, verifies the effective mount, and only then commits the authority, branch, and mount state to SQLite. Develop to Explore keeps the current branch and all working-tree changes while recreating the mount read-only. If a transition fails, Ayati restores the previous branch and mount; if recovery also fails, the workspace enters a failed state instead of reporting an authority it could not verify.

Deletion waits for canceled agent work to finish, validates that the recorded clone is exactly `<managed-root>/<workspace-id>/repo`, removes the owned sandbox, removes that workspace directory, and finally deletes its SQLite record. Foreign-key cascades remove its sessions and messages. Initialization must finish or fail before deletion so clone/setup work cannot recreate files after cleanup.

The container boundary includes:

- non-root image user;
- read-only container root;
- all Linux capabilities dropped and `no-new-privileges`;
- 256 PID, 2 GiB memory, and 2 CPU limits;
- private temporary and home tmpfs mounts;
- one repository bind mount, read-only for Explore and read-write for Develop;
- writable private `/tmp` and `/home/ayati` tmpfs mounts;
- a workspace-owned `/cache` bind outside the repository, preserved across container recreation;
- no Docker socket, host home, GitHub token, or model-provider key.

Commands run through `docker exec -i` and a fixed launcher with a two-minute timeout, 64 KiB command limit, 32 KiB bounds for each output stream, truncation reporting, and controller cancellation. The controller supplies fixed cache variables for Go, npm, pip, Cargo, and XDG-compatible tools. It decrypts the current workspace values for each command and sends shell-quoted exports over standard input; they are never Docker command arguments or permanent container environment. Setup receives only values explicitly marked for setup. Exact configured values are redacted from captured output before tool results are recorded, though transformed values cannot be reliably recognized. Network isolation is deferred because initial dependency installation requires network access.

Environment values use AES-GCM with a random local 256-bit key stored beside the database in a `0600` file. Names and exposure scope are readable metadata; API responses never include values. Mutations are rejected during initialization or an active agent run. Stop preserves values, while workspace deletion removes their rows through the existing foreign-key cascade. This protects against accidental repository, API, log, and Docker-metadata exposure, not against commands that are intentionally given the values.

## Agent Studio, execution, and authority

Agent Studio has a global catalog containing one immutable built-in Ayati agent, reusable custom agents, and reusable Markdown skills. Exactly one active agent is the global default. A custom agent can change identity, provider, model, instructions, step limit from 1 through 20, whether the shell tool is exposed, and an ordered list of at most twelve active skills. Archiving a non-default custom agent reassigns sessions using it to the current default. A skill cannot be archived while any agent still references it.

Agent definitions and skills do not store conversation or workspace context. Skill Markdown is inert prompt guidance stored in SQLite; browser import and export provide `.md` interchange without allowing executable skill scripts. At the start of a run, `internal/chat` loads the session's selected agent and its ordered skills, snapshots their revisions, resolves the agent's provider through the registry, resolves an empty model to that provider's private configured default, combines the non-overridable workspace prompt with custom instructions and skill Markdown, loads the session history, and runs the model. Editing an agent or skill affects future runs only. Historical assistant messages keep the producing agent, provider, model, and skill revision snapshot. Attachment count and combined Markdown size are bounded before storage.

Provider definitions are non-secret metadata. The registry exposes identity, protocol, capability flags, configured state, and the non-secret default model to the browser. Credentials remain in the private `0600` controller configuration file and are accepted write-only through mutation endpoints. Leaving the key field blank preserves an existing key; removal clears the saved connection and immediately disables runtime resolution. Fireworks, OpenAI, OpenRouter, Groq, Together AI, and DeepSeek are compiled provider specifications. The latter five reuse the bounded OpenAI-compatible protocol client while retaining fixed controller-owned endpoints and token-limit behavior. Connection verification calls the provider's models endpoint and returns only success or a bounded status error.

Model discovery is an on-demand controller operation for configured providers. `GET /api/providers/{id}/models` loads the saved key privately, calls only the specification's compiled endpoint, bounds and validates the response, sorts and deduplicates model IDs, and returns those non-secret IDs to the browser. Catalogs are not persisted or refreshed in the background, and provider and agent forms always retain manual model entry. Fireworks remains manual because its official catalog requires account context not stored by Ayati. Ayati does not load native libraries, scripts, downloaded packages, arbitrary endpoints, or arbitrary headers as provider plugins. Additional providers must register a compiled specification, implement the shared request contract, and pass the shell-call conformance tests.

The model receives exactly one function:

```json
{"name":"shell","arguments":{"command":"go test ./..."}}
```

There are no file, GitHub, service, lifecycle, or database tools exposed to the model. A shell-disabled agent receives no tools, and an unexpected shell call is rejected. A shell-enabled agent receives the one tool shown above. The web controller owns workspace creation, initialization, stopping, Git credentials, commits, pushes, and pull requests.

The composer has one Send action, a session-persisted agent selector, and a Stop action while that session owns the active run. Stop cancels the shared provider and shell context, preserves messages and activity already recorded, and moves the session to `canceled` instead of presenting a user decision as a failure. Discussion, planning, and review requests do not grant permission to modify files; the user must state an explicit implementation request. Explore additionally tells the model to research and propose changes without attempting mutations, while its read-only bind mount provides enforcement. Develop permits project mutations after an explicit implementation request. Every run receives the resolved project root, language/runtime/package-manager facts, baseline commit, preparation result, and detected verification commands. Custom instructions and skills remain subordinate to these controller rules. Publishing and authority changes remain unavailable through the model-facing shell.

## GitHub and publish boundary

GitHub OAuth state is kept in an HTTP-only, same-site callback cookie. The user access token is stored in a private local file. Repository selection is checked against repositories returned for the App installations before a workspace is created.

New-project creation uses the authenticated user's GitHub App token and requires the App's `Administration: write` repository permission. The controller asks GitHub to initialize the repository with a README, then uses GitHub's returned full name, clone URL, and default branch when entering the normal workspace pipeline. Private is the product default. The remote operation cannot be atomic with the local SQLite write, so Ayati reports partial failure and intentionally leaves the GitHub repository recoverable instead of deleting it automatically. GitHub credentials remain controller-only throughout this flow.

Authenticated clone and push use host Git with a short-lived private askpass script. The access token is passed only to that trusted Git child process and removed with the helper; it is never written into the remote URL or exposed to the model sandbox. Publishing stages all workspace changes, creates a focused user-supplied commit, pushes the working branch, and asks GitHub to open a draft pull request.

Mutating HTTP endpoints require the non-simple `X-Ayati-Request: 1` header. The server binds to `127.0.0.1:8080` by default. This is appropriate for the personal local prototype; remote hosting, multi-user sessions, webhook validation, installation-token brokerage, queues, and fleet scheduling are intentionally deferred.
