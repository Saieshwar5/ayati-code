# Execution Loops, Durable Runs, and Context Management

Status: design draft
Target: a reliable Perpetual execution-room subsystem for multiple users,
multiple agents, and tasks lasting seconds to hours.

## 1. Problem statement

Perpetual needs agent execution loops that:

- run for seconds, minutes, or hours;
- run while other users and other agents are also active;
- survive controller restarts, provider failures, rate limits, and timeouts;
- do not re-execute expensive work or duplicate side effects;
- keep context bounded and useful over very long conversations;
- keep user data isolated between tenants.

The current codebase already has two important primitives:

- `workspace_jobs` — a durable job table with queue, claim, lease, and retry state;
- `sessions` and `messages` — durable conversation storage.

The design should extend these rather than introduce a separate distributed
framework.

## 2. Best execution-loop shape

### 2.1 One loop = one durable state machine

Build each execution room as a six-state machine:

```text
queued -> claimed -> running -> waiting_user -> completed/failed/canceled
```

Each loop iteration is a **step**. A step is an atomic unit of work:

- model call;
- tool call (`shell(command)`);
- context compaction;
- pause for user input.

Persist:

1. step input before execution;
2. step output after success;
3. step idempotency key.

On restart:

1. reload the run;
2. replay completed step results from SQLite;
3. continue from the first incomplete step.

### 2.2 Event-sourced journal

The durable conversation log and the step log are the source of truth. Context
is a **derived view** built from that log. Never treat the in-memory prompt as
the authoritative state.

Proposed tables:

- `agent_runs`: run id, session id, workspace id, user id, state, model/prompt
  version, deadline, lease, heartbeat, current step, result, error.
- `agent_run_steps`: run id, step key, step kind, input JSON, output JSON,
  done_at, status. Primary key `(run_id, step_key)`.
- existing `messages`: durable conversation and tool-result history.
- `agent_work_memory`: compact scratchpad / notes / todo list per run.

### 2.3 Step wrapper

The rule from reliable long-task design is simple:

- check whether the step already completed;
- if yes, return its stored result;
- if no, run it once;
- write its result **only after success**;
- branch **only on logged values**, never on wall-clock time, `rand`, or live
  inputs.

This gives idempotency and resume with a tiny amount of code.

## 3. Where loops should run

### Recommendation: in the controller process, not in the microVM

Split the concepts:

| Component | Where it runs |
|---|---|
| Execution loop / brain | Controller process |
| Model/LLM calls | External provider, called by controller |
| Shell/tool execution | Workspace environment microVM |
| Durable state | SQLite beside the controller |
| Progress events | Controller -> browser SSE |

Reasons:

- the controller owns SQLite, leases, history, secrets, Git, and PR flow;
- the loop needs to be co-located with durable state to checkpoint cheaply;
- the microVM is untrusted execution substrate, not a place for durable brain;
- keeping brain and tool execution separate makes provider swaps easy;
- the existing `exec.Shell` contract is already one tool, so the loop can call
  through `Runtime.OpenShell(...)` without knowing whether it is local or cloud.

For Perpetual's current boundary, one Go process with a bounded worker pool of
goroutines is the right starting point.

## 4. Concurrency for multiple users and multiple agents

### 4.1 Worker pool, not goroutine-per-user without limit

- `agentRunner` owns a worker pool with a configured maximum:
  for example `maxConcurrentRuns = 8`.
- Each active run is one worker goroutine.
- New work is enqueued into SQLite, not launched directly from HTTP handlers.
- A dispatcher claims queued `agent_runs` and hands them to a free worker.

This avoids thousands of unmanaged goroutines and lets the system survive a slow
provider by backpressuring new runs.

### 4.2 Fairness and quotas

Initial policy:

- first-in, first-out by `created_at`;
- one active run per workspace;
- per-user active-run cap, for example 2;
- global active-run cap, for example 8.

The dispatcher checks these caps before claiming. Future priority/limits can be
added without changing the loop.

### 4.3 Leases and heartbeats

- each running run stores a short lease and heartbeat timestamp;
- worker updates the heartbeat every few seconds;
- on startup, `agent_runs` in `claimed` or `running` are marked interrupted;
- the user can retry from the last durable step or cancel.

This is the same pattern already used by `workspace_jobs`.

## 5. Handling runs of minutes and hours

### 5.1 Deadlines

Give each run:

- `step_timeout`, for example 2 minutes for a shell step, 60 seconds for a model
  call;
- `run_deadline`, for example 2 hours default and user configurable;
- `max_steps`, for example 200;
- `max_tokens`, for cost/context control.

When a deadline hits, the run transitions to `waiting_user` or `failed` with an
actionable reason.

### 5.2 Retry and circuit breakers

Classify errors:

Retry:

- 429 rate limits;
- 500/502/503/504 server errors;
- connection resets and timeouts.

Do not retry:

- 400 bad request;
- 401/403 authentication/authorization;
- context-overflow;
- policy/content refusal.

Retry with exponential backoff and jitter. Maintain a per-provider circuit
breaker so multiple users hitting the same outage do not amplify retries.

### 5.3 Checkpointing

Checkpoint after each meaningful unit, not on a timer:

- checkpoint the model response and tool result;
- update working notes and current step;
- keep checkpoint small;
- never checkpoint the entire model context.

This matches the existing `workspace_jobs` behavior and prevents expensive
model calls from being repeated after a restart.

### 5.4 Progress and observability

Emit bounded `run.changed` invalidation notices over the existing SSE stream.
The browser reloads authoritative run/message state from SQLite.

Persisted progress fields:

- current step/status;
- completed steps;
- current shell command;
- last error;
- deadline and remaining time.

Do not place prompt bodies or secret values in the event stream.

## 6. Context management

### 6.1 Separate memory store from prompt assembly

Keep two things separate:

- **Durable memory**: full conversation log, tool results, file/artifact
  references, and run-level scratch notes.
- **Prompt context**: a bounded, assembled view produced for one model call.

A common failure is stuffing the entire history into every prompt. Do not do
that.

### 6.2 Context layers

1. **Identity/system layer**

   - agent role;
   - `shell(command)` tool contract;
   - safety policy (shell only, no publishing without user authorization).

2. **Task/workspace layer**

   - repository and branch;
   - project root;
   - deterministic `EnvironmentSpec`;
   - current `git status`/diff summary;
   - currently changed paths.

3. **Conversation layer**

   - recent messages, bounded by token count;
   - tool call/result pairs kept in order;
   - oldest material summarized rather than dropped.

4. **Working memory layer**

   - current objective;
   - todo/progress notes;
   - decisions made;
   - file(s) being edited;
   - user authorizations.

5. **Tool output layer**

   - truncated/redacted shell output;
   - structured exit code, timeout flag, truncation flag.

### 6.3 Prompt assembly order

Recommended ordering in the model request:

1. system/identity/rules;
2. tool contract;
3. environment/repository state;
4. compaction summary and long-term notes;
5. recent rolling conversation window;
6. user message or agent continuation.

### 6.4 Compaction and token budget

- Keep a sliding window of the last N recent messages, for example 20.
- When the transcript grows beyond a threshold, summarize older turns into a
  compact "history summary".
- Store the summary idempotently by transcript cursor.
- Add a token-budget allocator with explicit caps, for example:

```text
system + tools        25%
workspace state       20%
working memory        20%
recent messages       30%
overhead/response      5%
```

- Do not compact silently. Record the compaction as a `compact` step so the
  durable transcript remains explainable.
- Keep tool results as facts with file/path and exit status; truncate output to
  the existing bounded shell limits.

### 6.5 Where context lives

- `messages` = durable transcript.
- `agent_run_steps` = durable step/result journal.
- `agent_work_memory` = small JSON scratchpad, updated after each step.
- `workspace_profiles` / environment spec = stable repo context.
- No secret values in context. Workspace environment values are decrypted only
  at shell-execution time and best-effort redacted from recorded output.

## 7. Prompts and tooling

Keep the tool set small. The planned agent has one tool: `shell(command)`.
That constraint is a feature because it simplifies loops, security, and
checkpointing.

Prompt principles:

- deterministic prompt templates with a version stored on the run;
- one tool signature, one clear stop condition;
- explicit user authorization boundary;
- tool results are bounded and redacted;
- model is told the current step, working notes, and recent history only.

Run-control signals:

- `finish` or equivalent at the model/loop layer;
- `max_steps`, `deadline`, `max_tokens`;
- user Stop/Cancel cancels the whole run and its in-VM process group;
- user Pause moves the run to `waiting_user`.

## 8. Security and multi-tenancy

- every run row is scoped by `user_id`;
- every run references one `session` and one `workspace`, both owner-checked;
- one active run per workspace avoids two agents mutating the same files;
- token/provider secrets remain controller-side and never go to the microVM;
- checkpoints never include raw workspace environment values.

## 9. Scaling path

Start with this simple, reliable shape:

```text
HTTP enqueue -> SQLite durable run ->
controller in-process dispatcher + bounded worker pool ->
provider + Runtime shell tool ->
SQLite step journal ->
SSE progress
```

Do **not** add Temporal, Kafka, Postgres queues, Redis, or a worker fleet first.
The current product is one local-first Go process with SQLite. The durable
job/lease primitives already give 80% of what long-running agent execution
needs.

If later the controller must exceed one machine/process:

1. split the runner into a separate `cmd/perpetual-runner` binary;
2. retain SQLite/WAL as the shared durable store, or move to Postgres only when
   justified;
3. keep the runner protocol identical to the in-process worker;
4. treat execution rooms as external consumers of the same durable run tables.

## 10. Build order

1. Add durable `agent_runs` and `agent_run_steps`.
2. Implement dispatcher + bounded worker pool in the controller.
3. Implement step wrapper and run resume.
4. Reconnect chat to a no-op/planning loop that issues `shell(command)`.
5. Add cancellation, deadlines, heartbeats, retry classification, and jitter.
6. Add context assembler with rolling window + summary.
7. Add token budget and compaction.
8. Add multi-user quotas and event progress.
9. Harden with tests using injected HTTP transports and a fake runtime; do not
   require real AWS/model/network in tests.

## Sources

Reviewed in depth:

- You Don't Need Temporal Yet: Durable Execution for AI Agents in 150 Lines
  https://hackernoon.com/you-dont-need-temporal-yet-durable-execution-for-ai-agents-in-150-lines
- How to Handle Long-Running Tasks in AI Agents (2026), Fastio
  https://fast.io/resources/ai-agent-long-running-tasks

Search results consulted:

- LLM Prompt Best Practices for Large Context Windows
  https://winder.ai/llm-prompt-best-practices-large-context-windows/
- Context budgeting (token economy)
  https://visdom-maturity-matrix.virtuslab.com/guides/development/context-budgeting-token-economy
- Advanced Context Engineering: Semantic Prompt Caching & Token Optimization
  https://akmalkhaniub.github.io/blog/context-engineering-prompt-caching.html
- Temporal Review (2026): Durable Execution Platform
  https://tooldirectory.ai/tools/temporal
