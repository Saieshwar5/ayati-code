# Ayati

Ayati is a deliberately small Linux coding-agent harness. It uses one Fireworks model, one `shell` tool, one sequential loop, and plain JSONL sessions.

> Ayati currently runs shell commands directly with your Linux user permissions. Use it only with trusted prompts and trusted local projects.

## Requirements

- Linux
- Go 1.25 or newer
- A Fireworks API key
- A Fireworks model identifier

## Run

```bash
export FIREWORKS_API_KEY="..."
export AYATI_MODEL="accounts/fireworks/models/your-model"
go run -buildvcs=false ./cmd/ayati
```

Run against another project:

```bash
go run -buildvcs=false ./cmd/ayati --workspace /path/to/project
```

Resume a session:

```bash
go run -buildvcs=false ./cmd/ayati --workspace /path/to/project --session 1a2b3c4d
```

`--model` overrides `AYATI_MODEL` for a new session. Resumed sessions always use their stored model.

## Terminal commands

- `/new` starts an empty session with the current model.
- `/sessions` lists sessions for the current workspace.
- `/resume <id>` resumes by full ID or unique prefix.
- `/help` shows commands.
- `/exit` quits.

## Behavior

- The model receives exactly one function tool: `shell`.
- A request may use at most 20 model decisions.
- Each decision may contain at most one shell call.
- Shell commands start in the selected workspace.
- Commands have a two-minute timeout, 64 KiB input limit, and bounded stdout/stderr.
- Sessions are append-only JSONL files under `$XDG_STATE_HOME/ayati/sessions` or `~/.local/state/ayati/sessions`.
- The complete session is replayed. There are no snapshots, compaction, provider switching, model catalogs, modes, or background services.

## Verify

```bash
go test -buildvcs=false ./...
go vet -buildvcs=false ./...
CGO_ENABLED=0 go build -buildvcs=false -trimpath -o ayati ./cmd/ayati
```

See [docs/architecture.md](docs/architecture.md) for the small set of runtime boundaries.
