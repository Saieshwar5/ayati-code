# No-Nonsense Coding AI

`ayati-micro` is a tiny terminal coding agent written in Go. It has one
provider (Fireworks), one model-visible tool (`shell`), and persistent JSONL
sessions. The implementation uses only the Go standard library.

## Build and configure

Requirements: Go 1.22 or newer, `/bin/sh`, and a Fireworks API key.

```sh
cd /home/sai-eshwar/my_folder/ayati-micro
go build -o ayati-micro ./cmd/ayati-micro
./ayati-micro setup
```

`setup` securely prompts for the API key and model. It saves them in the
`.env` beside the real `ayati-micro` executable:

```dotenv
FIREWORKS_API_KEY="your-key"
NCA_MODEL="accounts/fireworks/models/deepseek-v4-flash-0731"
```

The file is created with owner-only `0600` permissions and is ignored by Git.
`.env.example` documents the supported format without containing a secret.
Exported environment variables override values from `.env`.

## Install the command

```sh
./ayati-micro install
```

This creates the symlink `~/.local/bin/ayati-micro` pointing to the built
binary. If necessary, add the directory to your shell PATH:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

You can then open the agent from any coding project:

```sh
cd /path/to/project
ayati-micro
```

The directory where you launch it is the coding workspace. Configuration is
still loaded from the agent installation directory.

## Usage

```sh
ayati-micro                         # continue this project's latest session
ayati-micro new                     # create a new session
ayati-micro continue                # continue the latest project session
ayati-micro sessions                # list sessions
ayati-micro open <session-id>       # open a saved session
ayati-micro -cwd /path/to/project   # use a specific coding directory
ayati-micro setup                   # configure key and model
ayati-micro config show             # show masked configuration
ayati-micro config key              # replace the saved API key
ayati-micro model                   # show the default model
ayati-micro model <model-id>        # save another default model
ayati-micro install                 # install the user-local command
```

Flags must appear before commands. `-model MODEL_ID` overrides the model for
one invocation without changing `.env`.

Interactive commands:

```text
/new                 create a new session
/sessions            list sessions
/open ID             open a session
/session             show current session details
/model               show the active model
/model MODEL         change model for this process
/model save MODEL    change and persist the default model
/compact             summarize older context now
/help                show commands
/quit                exit
```

## Default model

The default is:

```text
accounts/fireworks/models/deepseek-v4-flash-0731
```

Change it permanently:

```sh
ayati-micro model accounts/fireworks/models/kimi-k2p6
```

Or temporarily:

```sh
ayati-micro -model accounts/fireworks/models/kimi-k2p6
```

## Sessions and context

Sessions are append-only JSONL files under `~/.nca/sessions`. Every user
message, assistant response, shell call, and shell result is persisted
immediately. Sessions are scoped by their absolute coding directory.

The full exact history always remains on disk. The active provider context is:

```text
system prompt
+ latest rolling summary, when one exists
+ recent exact conversation and shell activity
+ exact current user request
```

The agent asks Fireworks for the selected model's `contextLength` and uses 70%
as its safe input budget. If metadata is unavailable, DeepSeek V4 uses a
1,048,576-token fallback and unknown models use 128,000 tokens. Token usage is
estimated conservatively from the serialized request.

Up to 100 recent shell call/result pairs remain exact. When the safe token
budget or that count is reached, the agent asks the same Fireworks model for
one tool-free summary, appends the summary checkpoint to the JSONL file, and
continues with the summary plus recent exact activity. `/compact` invokes the
same mechanism manually.

The current user request is never removed. If a summarized historical detail
is needed, the model is told the exact session path and can use its existing
`shell` tool to search a bounded part of the JSONL file. No history-reading
tool is added.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `FIREWORKS_API_KEY` | required | Fireworks authentication |
| `NCA_MODEL` | DeepSeek V4 Flash 0731 | Model ID |
| `NCA_SESSION_DIR` | `~/.nca/sessions` | Session storage |
| `NCA_CONTEXT_PERCENT` | `70` | Percentage of provider context allowed for input |
| `NCA_MODEL_CONTEXT_TOKENS` | model-aware fallback | Fallback if metadata lookup fails |
| `NCA_MAX_CONTEXT_TOOL_PAIRS` | `100` | Recent exact shell call/result pairs |
| `NCA_MAX_TOOL_CALLS` | `30` | Shell calls allowed per user input |
| `NCA_SHELL_TIMEOUT` | `2m` | Timeout for each shell call |
| `NCA_MAX_OUTPUT` | `32768` | Maximum stdout/stderr characters each |
| `NCA_FIREWORKS_URL` | Fireworks chat completions URL | Endpoint override |
| `AYATI_MICRO_ENV` | executable-adjacent `.env` | Config path override |

## Security

The API key is used by the Fireworks provider but is removed from the
environment passed to model-issued shell commands. Configuration displays mask
the key, and session records do not intentionally contain it.

The model still has shell authority in the selected coding directory. It can
modify files, run programs, access other credentials available to the process,
and invoke Git. Run it only where that authority is acceptable.

## Verify

```sh
go test ./...
go vet ./...
go build -o ayati-micro ./cmd/ayati-micro
```
