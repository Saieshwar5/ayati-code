# Ayati Runtime

Ayati is a small, one-shot coding-agent runtime for disposable Linux virtual
machines. Each process accepts one JSON request, runs one bounded model/shell
loop, emits JSONL events, returns one terminal outcome, and exits.

The model sees exactly one tool:

```text
shell(command)
```

There is no interactive UI, session discovery, installer, database, plugin
system, planner, reviewer, sub-agent, or automatic provider fallback. The Go
implementation uses only the standard library.

## Runtime contract

```text
validate request
      |
      v
ask model ---- final text ----> completed
      |
      v
validate one shell call
      |
      v
execute with time/output bounds
      |
      v
record result and ask model again
      |
      +---- context pressure ----> checkpoint, fresh context, continue
      |
      +---- work limit ---------> tool-disabled final handoff, exhausted
```

One work decision can contain at most one shell call. `max_steps` bounds these
tool-enabled decisions and therefore also bounds shell calls. If the last step
uses the shell, Ayati first records the result, then reserves one tool-disabled
model call for a truthful final handoff. The outcome remains `exhausted`, so a
useful final response never falsely means that the task completed.

## Build

Requirements are Go 1.22+, Linux, and `/bin/bash` in the target VM.

```sh
go test ./...
CGO_ENABLED=0 go build -trimpath -o ayati-runtime ./cmd/ayati-runtime
```

The binary has no database, home-directory, or configuration-writing
dependency. Copy it and a JSON configuration file into the VM.

## Run

Set the API key named by `provider.api_key_env`, then pass one request on
standard input:

```sh
export AYATI_API_KEY="your-short-lived-key"
./ayati-runtime run --config examples/fireworks.json < examples/request.json
```

A request contains only run identity, the task, and the prepared workspace:

```json
{
  "version": 1,
  "run_id": "job-123",
  "prompt": "Fix the failing tests and verify the result.",
  "workspace": "/workspace"
}
```

`run_id` may be omitted; the command generates one. `workspace` must be an
existing absolute directory.

## Configuration

```json
{
  "version": 1,
  "provider": {
    "kind": "openai-chat",
    "model": "accounts/fireworks/models/deepseek-v4-flash-0731",
    "endpoint": "https://api.fireworks.ai/inference/v1/chat/completions",
    "api_key_env": "AYATI_API_KEY",
    "max_output_tokens": 8192,
    "context_window_tokens": 1048576
  },
  "limits": {
    "max_steps": 30,
    "max_context_rollovers": 2,
    "run_timeout_seconds": 1800,
    "model_timeout_seconds": 300,
    "shell_timeout_seconds": 120,
    "max_tool_output_bytes": 16384
  },
  "shell": {
    "path": "/bin/bash"
  }
}
```

Defaults:

| Setting | Default |
| --- | ---: |
| `provider.api_key_env` | `AYATI_API_KEY` |
| `provider.max_output_tokens` | `8192` |
| `provider.context_window_tokens` | disabled when omitted |
| `limits.max_steps` | `30` |
| `limits.max_context_rollovers` | `2` |
| `limits.run_timeout_seconds` | `1800` |
| `limits.model_timeout_seconds` | `300` |
| `limits.shell_timeout_seconds` | `120` |
| `limits.max_tool_output_bytes` | `16384` for each stdout/stderr stream |
| `shell.path` | `/bin/bash` |

Omitted versions currently default to `1`. Unknown fields, unsupported
versions, and negative limits are rejected rather than ignored.

Set `provider.context_window_tokens` to the documented window for the selected
model to enable in-run context rollover. Ayati checkpoints after a shell result
when estimated pressure reaches 70% of the usable window (the configured window
minus output-token and safety reserves). It prefers provider-reported input
usage and also maintains a conservative local estimate. The checkpoint call has
no tools. Ayati then starts a fresh provider conversation containing the exact
original request plus the factual checkpoint and continues with the remaining
work steps.

Checkpoint and finalization calls do not consume `max_steps`; they are counted
in `model_calls`. `max_context_rollovers` prevents endless summarization. If it
is reached, the runtime emits a final handoff and exits `exhausted`.

This rollover policy intentionally does not preflight or rewrite the initial
user request. If the initial prompt itself exceeds the provider limit, the
provider error is returned through the normal failed-run path.

Context-window values are model-specific deployment inputs; the runtime does
not discover them. Verify each example value against the selected model's
documentation before deployment and update it whenever the model changes.

### Providers

`openai-chat` uses the Chat Completions tool-call protocol. Its endpoint is
required and configurable, so it supports Fireworks and tested compatible
endpoints without making the runtime Fireworks-specific.

`openai-responses` uses the native OpenAI Responses protocol. Its default
endpoint is:

```text
https://api.openai.com/v1/responses
```

`anthropic` uses the native Anthropic Messages content-block protocol. Its
default endpoint is:

```text
https://api.anthropic.com/v1/messages
```

Each run fixes one provider and model. Ayati never changes providers or models
halfway through a tool history.

## Events and outcomes

Standard output contains only JSONL events. A normal tool round resembles:

```json
{"version":1,"seq":1,"type":"run.started","run_id":"job-123","provider":"openai-chat","model":"...","prompt":"...","workspace":"/workspace","limits":{"max_steps":30}}
{"version":1,"seq":2,"type":"model.decision","run_id":"job-123","phase":"work","step":1,"decision":{"shell_call":{"command":"go test ./..."}}}
{"version":1,"seq":3,"type":"tool.started","run_id":"job-123","phase":"work","step":1,"command":"go test ./..."}
{"version":1,"seq":4,"type":"tool.completed","run_id":"job-123","phase":"work","step":1,"tool_result":{"stdout":"ok\n","exit_code":0,"duration_ms":734}}
```

Exactly one terminal event is emitted:

- `run.completed`
- `run.exhausted`
- `run.failed`
- `run.cancelled`

The terminal event contains step count, shell-call count, elapsed time, final
text or failure reason, provider/model call counts, context-rollover count, and
provider token usage when reported. `finalized: true` means a special
tool-disabled final handoff succeeded. A nonzero shell exit is an observation
returned to the model, not a runtime failure.

`context.checkpoint` records each generated checkpoint. `run.finalizing`
records the reserved handoff attempt. If that provider call fails or returns no
text, Ayati still emits a deterministic fallback explaining the stop reason and
points to the exact shell output already preserved in JSONL.

Provider or startup diagnostics use standard error. The process exits zero only
for `completed`.

## Shell execution

The shell executes `/bin/bash -lc` in the request workspace. Standard output
and error are bounded while the process is running; Ayati retains the head and
tail and reports original byte counts and truncation flags. The command is not
duplicated in the tool result sent back to the model.

Timeout or cancellation kills the Linux process group rather than only the
immediate Bash process.

By default, the child receives a small environment containing common runtime
variables such as `PATH`, `HOME`, locale, and language cache paths. It does not
inherit arbitrary parent variables. `shell.pass_env` can replace this default
with an explicit list, but it cannot include the configured provider-key
variable.

## VM deployment

Use one process for one task:

1. Create a disposable VM from a prepared image.
2. Mount or clone the repository at `/workspace`.
3. Inject a short-lived provider credential.
4. Start `ayati-runtime run` with JSON on stdin.
5. collect stdout JSONL and workspace artifacts.
6. Destroy the VM.

A practical base image needs the runtime plus Bash, Git, ripgrep, coreutils,
and CA certificates. Add Node, Go, Python, or Java in separate language images;
the runtime itself does not install development environments.

Do not give the worker VM host home directories, container sockets, cloud
metadata credentials, or long-lived provider keys. An unrestricted shell and a
same-identity in-process secret are not a strong security boundary. For
stronger deployments, use short-lived credentials or an external inference
gateway and enforce network policy outside Ayati.

## Repository layout

```text
cmd/ayati-runtime/       process entrypoint and dependency wiring
internal/runtime/        bounded state machine and semantic contracts
internal/provider/       native provider protocol adapters
internal/shell/          Linux process execution and bounded capture
internal/protocol/       strict JSON input/config and JSONL events
```

The runtime package is intentionally divided by lifecycle responsibility:

```text
loop.go       tool-enabled work loop
context.go    context policy, pressure, checkpoint, and rollover
outcome.go    finalization, terminal results, and event sequencing
limits.go     runtime limit defaults and event snapshots
request.go    runtime request validation
events.go     event and terminal-result contracts
types.go      model, conversation, shell, and usage contracts
prompt.go     system, checkpoint, continuation, and final prompts
```

## Verify

```sh
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o /tmp/ayati-runtime ./cmd/ayati-runtime
```
