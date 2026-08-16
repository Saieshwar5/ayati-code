# Ayati local architecture

## Product boundary

Ayati is a single-user application running on one Linux machine. One Go process serves the browser UI, calls GitHub and Fireworks, owns SQLite, and controls local Docker containers. It deliberately has no Postgres, VM manager, worker fleet, queue, background agent service, or multi-provider abstraction.

The durable product object is a workspace containing one or more sessions:

```text
GitHub repository + base/working branch
  -> local clone + SQLite record
  -> one named Docker sandbox
  -> dependency initialization
  -> separate durable session chats and explicit agent runs
  -> Git status/diff review
  -> commit, push, and draft pull request
  -> user stops sandbox
```

The repository and SQLite record survive a normal Stop. A ready workspace restores its named container if the controller process or container was restarted. Delete is a separate confirmed action that removes the managed container, local clone, workspace record, sessions, and messages; it never deletes the remote GitHub branch or pull request.

## Component ownership

- `cmd/ayati` owns signal handling and selects the web server or the small `config` command.
- `web` owns the React and TypeScript browser interface, its component tests, and the Vite build.
- `internal/webapp` owns HTTP routes, the embedded production bundle, local server startup, and component wiring.
- `internal/workspace` owns the SQLite schema, workspace state, trusted host Git operations, setup detection, change inspection, and publish flow.
- `internal/sandbox` owns persistent Docker-container creation, restoration, removal, and bounded shell execution.
- `internal/githubapp` owns GitHub user authorization, installed-repository discovery, branch listing, draft pull requests, and the private credential file.
- `internal/chat` binds each durable session conversation to the agent loop and permits only one active run per workspace.
- `internal/agent` owns the one-tool prompt, shared messages, and sequential 20-decision loop.
- `internal/fireworks` owns the single Fireworks request format.
- `internal/config` owns private Fireworks configuration and its terminal setup command.

Infrastructure packages do not depend on `internal/webapp`; the web layer connects consumer-owned interfaces.

The React application calls the existing JSON endpoints and does not own durable state or runtime
lifecycle. Its production build is written to `internal/webapp/dist` and embedded into the Go
binary. Development may use Vite's local proxy, but deployment remains one Go process; there is no
Node.js production server.

## SQLite state

SQLite uses WAL mode, foreign keys, a five-second busy timeout, and one database connection. The schema contains:

- `workspaces`: repository, branch, authority, effective mount mode, local path, sandbox name, setup command, lifecycle status, failure, and pull-request identity.
- `sessions`: workspace-scoped conversations with independent titles, run status, failure, and timestamps.
- `messages`: ordered full agent messages, including tool calls and tool results, linked to a session.
- `workspace_environment`: encrypted workspace-scoped values, variable names, setup exposure, and timestamps.

Workspace lifecycle values describe only the environment: `creating`, `initializing`, `initialization_failed`, `ready`, and `stopped`. Session lifecycle values describe agent work: `idle`, `working`, `review`, and `failed`. Existing workspace conversations are migrated into an `Original session` without losing messages.

Sessions share one workspace clone, branch, sandbox, environment, and diff. They isolate conversational context and activity history, not filesystem state. The controller rejects a second agent run while another session in the same workspace is active. Changes, environment, and publishing are therefore workspace-scoped, while conversation and internal activity are session-scoped.

## Sandbox lifecycle

Initialization first clones or opens the repository with trusted host Git. A requested new working branch is created only in the local clone. Ayati then creates `ayati-workspace-<id>` with a writable preparation mount and runs dependency setup through the same bounded shell contract later used by the agent. Before an Explore workspace becomes ready, the controller removes that container, recreates `/workspace` read-only, verifies Docker's effective mount metadata, and records it. Develop remains read-write. The ready container stays alive across chat turns and controller restarts; `Stop` removes only that validated Ayati container name.

Deletion waits for canceled agent work to finish, validates that the recorded clone is exactly `<managed-root>/<workspace-id>/repo`, removes the owned sandbox, removes that workspace directory, and finally deletes its SQLite record. Foreign-key cascades remove its sessions and messages. Initialization must finish or fail before deletion so clone/setup work cannot recreate files after cleanup.

The container boundary includes:

- non-root image user;
- read-only container root;
- all Linux capabilities dropped and `no-new-privileges`;
- 256 PID, 2 GiB memory, and 2 CPU limits;
- private temporary and home tmpfs mounts;
- one repository bind mount, read-only for Explore and read-write for Develop;
- writable private `/tmp`, `/home/ayati`, and `/cache` tmpfs mounts;
- no Docker socket, host home, GitHub token, or Fireworks key.

Commands run through `docker exec -i` and a fixed launcher with a two-minute timeout, 64 KiB command limit, 32 KiB bounds for each output stream, truncation reporting, and controller cancellation. The controller decrypts the current workspace values for each command and sends shell-quoted exports over standard input; they are never Docker command arguments or permanent container environment. Setup receives only values explicitly marked for setup. Exact configured values are redacted from captured output before tool results are recorded, though transformed values cannot be reliably recognized. Network isolation is deferred because initial dependency installation requires network access.

Environment values use AES-GCM with a random local 256-bit key stored beside the database in a `0600` file. Names and exposure scope are readable metadata; API responses never include values. Mutations are rejected during initialization or an active agent run. Stop preserves values, while workspace deletion removes their rows through the existing foreign-key cascade. This protects against accidental repository, API, log, and Docker-metadata exposure, not against commands that are intentionally given the values.

## Agent and authority

The model receives exactly one function:

```json
{"name":"shell","arguments":{"command":"go test ./..."}}
```

There are no file, GitHub, service, lifecycle, or database tools exposed to the model. The web controller owns workspace creation, initialization, stopping, Git credentials, commits, pushes, and pull requests.

The composer has one Send action. Discussion, planning, and review requests do not grant permission to modify files; the user must state an explicit implementation request. Explore additionally tells the model to research and propose changes without attempting mutations, while its read-only bind mount provides enforcement. Develop permits project mutations after an explicit implementation request. Publishing and authority changes remain unavailable through the model-facing shell.

## GitHub and publish boundary

GitHub OAuth state is kept in an HTTP-only, same-site callback cookie. The user access token is stored in a private local file. Repository selection is checked against repositories returned for the App installations before a workspace is created.

Authenticated clone and push use host Git with a short-lived private askpass script. The access token is passed only to that trusted Git child process and removed with the helper; it is never written into the remote URL or exposed to the model sandbox. Publishing stages all workspace changes, creates a focused user-supplied commit, pushes the working branch, and asks GitHub to open a draft pull request.

Mutating HTTP endpoints require the non-simple `X-Ayati-Request: 1` header. The server binds to `127.0.0.1:8080` by default. This is appropriate for the personal local prototype; remote hosting, multi-user sessions, webhook validation, installation-token brokerage, queues, and fleet scheduling are intentionally deferred.
