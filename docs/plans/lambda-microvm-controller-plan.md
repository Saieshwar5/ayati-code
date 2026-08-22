# Perpetual Controller: Lambda MicroVM Execution Environments — Master Plan

Status: research-backed design draft
Substrate phase 1: AWS Lambda MicroVMs
Substrate phase 2 (future): self-managed Firecracker microVMs

## 1. Purpose

This document captures the master plan for moving Perpetual's workspace and
agent execution from the bounded local shell into isolated microVM environments.
The controller remains one small, local-first Go process. AWS Lambda MicroVMs
is the first execution substrate; a later self-managed Firecracker provider can
replace or complement it behind the same control-plane seam.

## 2. Confirmed research facts

Verified by searching and fetching current sources with `parallel-cli` plus the
AWS notes in `aws/`.

### AWS Lambda MicroVMs

- Powered by Firecracker; serverless, VM-level isolation, stateful snapshot-based
  startup, suspend/resume with intact memory and disk.
- Image build flow: zip containing a `Dockerfile` plus app files in S3 ->
  `CreateMicrovmImage` -> Lambda builds and boots the application -> captures a
  snapshot.
- Runtime flow: `RunMicrovm` returns a unique HTTPS endpoint
  (`<microvm-id>.lambda-microvm.<region>.on.aws`).
- Data-plane ingress:
  - default proxy port: 8080 inside the VM
  - per-request override: `X-aws-proxy-port`
  - authentication: short-lived JWE token in `X-aws-proxy-auth`
  - token max TTL: 60 minutes
  - protocols: HTTP/1.1, HTTP/2, gRPC, WebSockets
- Lifecycle hooks are HTTP endpoints in the guest and must listen on port 9000:
  `/aws/lambda-microvms/runtime/v1/{ready,validate,run,resume,suspend,terminate}`.
- Lifecycle states:
  `PENDING -> RUNNING -> SUSPENDING -> SUSPENDED -> TERMINATING -> TERMINATED`.
- Suspended microVMs incur zero compute cost; only snapshot storage is charged.
- The AWS SDK for Go v2 already provides
  `github.com/aws/aws-sdk-go-v2/service/lambdamicrovms`.
- Real-world provider constraints:
  - API rate limits: RunMicrovm <= 5 TPS, ResumeMicrovm <= 5 TPS,
    SuspendMicrovm <= 2 TPS, TerminateMicrovm <= 10 TPS,
    CreateMicrovmAuthToken <= 50 TPS.
  - `maxIdleDurationSeconds` must be >= 60.
  - maximum microVM duration is 8 hours.
  - hooks port must be 9000; data-plane port defaults to 8080.
  - bind guest listeners to 0.0.0.0; localhost-only listeners are not reachable.
  - image-level environment variables are shared across all instances and must
    not contain secrets.
- Approximate cost at default 1 vCPU / 2 GiB:
  `$0.0997/vCPU-hr + $0.0132/GB-hr` while running, zero compute while suspended,
  and `$0.08/GB/month` snapshot storage.

### Firecracker

- Open-source Rust VMM using Linux KVM.
- Boots microVMs in ~125 ms with < 5 MiB overhead per VM.
- The `jailer` adds cgroups, namespaces, seccomp, and chroot defense-in-depth.
- Self-managing Firecracker means owning guest kernels, root filesystems, TAP/CNI
  networking, snapshot storage, scheduling, and authentication.
- This is valuable later for cost at scale, long sessions, custom networking, or
  avoiding AWS service lock-in.

## 3. Product model

An environment is a reusable microVM image/type. An environment instance is one
live microVM. A workspace binds to an instance. An execution room is an agent
loop that owns one shell connection into that instance. The controller keeps Git
and GitHub authority and treats the microVM as the isolated working directory.

Recommended first deployment model:

- One microVM instance per workspace.
- Multiple sessions/execution rooms may be visible, but one active room executes
  at a time.
- This preserves the existing property that sessions share one repository,
  branch, cache, and diff.

## 4. Controller blocks

| Block | Responsibility | Current / recommended home |
|---|---|---|
| Authentication / users | GitHub OAuth, sessions, encrypted credentials | `internal/accounts` + `internal/githubapp` |
| Workspaces | repo/branch selection, SQLite workspace state, trusted Git, PR | `internal/workspace` |
| Context management | durable conversations, assembled working context | `internal/workspace` messages + future context builder |
| Execution rooms | agent loops, single shell tool, run lifecycle, cancel | future `internal/execution` or `internal/agent` |
| Environments | image build, microVM create/run/suspend/resume/terminate, repo sync | new `internal/environments` |
| Controller core | composition root, durable job worker, runtime provider selection, recovery | `internal/webapp` wiring + `internal/workspace` worker |
| Bounded shell | single command execution contract | existing `internal/exec` |

## 5. Most important architectural move

The existing seams already match the microVM design and should be preserved:

```go
type Runtime interface {
    Start(context.Context, Ref) error
    Stop(context.Context, Ref) error
    OpenShell(context.Context, Ref, map[string]string) (exec.Shell, error)
    Destroy(context.Context, Ref) error
}

type Shell interface {
    Execute(context.Context, ShellRequest) ShellResult
}
```

For Lambda MicroVMs:

- `Start` -> `RunMicrovm`
- `Stop` -> `SuspendMicrovm`
- `Destroy` -> `TerminateMicrovm`
- `OpenShell` -> returns an HTTP-backed `exec.Shell` that talks to an in-VM agent

This keeps the agent's one tool (`shell(command)`) unchanged across execution
substrates.

## 6. In-VM agent contract

Build a small `cmd/vmagent` binary inside every Perpetual microVM image.

Listeners:

- 8080: application/data plane
- 9000: Lambda lifecycle hooks

Data-plane endpoints:

- `GET /healthz`
- `POST /v1/bootstrap` — receive workspace tarball, environment values, workdir
- `POST /v1/exec` — execute one bounded `/bin/sh -c` command
- `GET /v1/tar` — stream the workspace working tree back to the controller
- Later: `POST /v1/sessions` for streamed command output

`/v1/exec` should use the same shapes as `exec.ShellRequest` and
`exec.ShellResult` so the controller contract stays one tool with one shape.

The controller never sends Git/GitHub credentials to the VM.

## 7. Repository sync model

Do not clone with GitHub credentials inside the microVM.

1. Controller clones/opens the repository with trusted host Git.
2. Controller creates or resumes the microVM.
3. Controller prepares a tarball of the working tree.
4. Controller calls `/v1/bootstrap` with the tarball.
5. VM agent extracts to `/workspace`.
6. Dependency install and verification run through `/v1/exec`.
7. For review/publish, controller calls `/v1/tar`, materializes a local working
   tree, and runs trusted host Git commit/push/PR.

This preserves the rule that commits, pushes, and pull requests remain
controller-owned actions.

## 8. Image building

Build two levels of images:

1. Generic Perpetual agent image:
   - `vmagent` binary
   - base Go, Node, and Python toolchains
   - no repository content and no secrets
2. Environment-version image, built from the deterministic `EnvironmentSpec`
   fingerprint, mirroring the existing `project_environments` /
   `environment_versions` model.

Build artifacts must live in an S3 bucket in the same region as the image.

## 9. End-to-end user flow

1. Sign in through GitHub OAuth.
2. Select repository and branch.
3. Create workspace:
   - enqueue durable `prepare_workspace` job
   - clone and analyze with trusted host Git
   - compute deterministic `EnvironmentSpec` and bind an environment version
4. Create environment instance:
   - find/build a microVM image for the spec
   - `RunMicrovm`
   - persist `runtime_ref = microvmId` and `runtime_state = running`
   - bootstrap repo tarball and setup-only environment values
5. Prepare workspace:
   - run setup/verify through `/v1/exec`
   - record and redact output
6. Agent work:
   - execution room starts per session
   - agent uses the single `shell(command)` tool
   - each tool call becomes `POST /v1/exec`
   - results are stored in `messages`
7. Idle:
   - `Stop` or idle policy calls `SuspendMicrovm`
   - zero compute while suspended; state preserved
   - `Resume` issues a fresh auth token and reconnects
8. Review/publish:
   - controller fetches `/v1/tar`
   - materializes the tree locally
   - trusted host Git diff/commit/push/PR
9. Delete:
   - `TerminateMicrovm`
   - delete local workspace data and SQLite state
   - never delete the remote GitHub branch/PR

## 10. Context management

Four layers, assembled at turn time, not one big stored blob:

1. Identity/workspace context — user, repo, branch, project root,
   `EnvironmentSpec`.
2. Conversation context — `sessions` and `messages` (already durable).
3. Working context — current `git status`, diff summary, changed paths, last
   result (recomputed each turn).
4. Agent scratch context — small per-room working memory/artifacts.

Keep stored context bounded. Do not put full repo contents or full shell logs
into in-memory context rows.

## 11. Execution rooms

- One execution room = one durable session with a run lease.
- Lifecycle: `idle -> working -> review/failed/canceled`.
- Loop:
  1. assemble context
  2. plan/choose next step
  3. call `shell(command)`
  4. write tool call and result into `messages`
  5. repeat until complete, stopped, timeout, or user cancel
- Enforce one active room per workspace using the existing durable job/lease
  pattern.
- Stop/cancel must kill the in-VM process group and cancel the HTTP request.

## 12. Reliability rules

1. Keep providers behind the existing `Runtime` seam.
2. Persist durable state before external I/O.
3. Make lifecycle operations idempotent.
4. Refresh short-lived AWS data-plane tokens before use.
5. Add retry/backoff/jitter for AWS API rate limits.
6. Reuse `workspace_jobs` for environment and agent operations.
7. Preserve bounded output and redaction in the remote shell.
8. Keep GitHub credentials controller-only.
9. Send workspace environment values only to the authenticated VM endpoint;
   purge on suspend and re-send after resume.

## 13. Future Firecracker provider

Defer until Lambda MicroVMs is stable and cost/limits are quantified. The
migration lever is the stable guest contract: the same `vmagent`, `/bootstrap`,
`/exec`, `/tar`, and `/healthz` endpoints run unchanged in a self-managed
Firecracker guest.

Firecracker provider responsibilities:

- EC2/KVM host pool or single Linux host
- `firecracker` + `jailer` execution
- guest kernel + root filesystem builder
- TAP/CNI networking or private controller networking
- snapshot to local disk or S3
- lifecycle and authentication layer

## 14. Build order

1. **Phase 0 — research spike**
   - Add `github.com/aws/aws-sdk-go-v2/service/lambdamicrovms`.
   - Run one real `RunMicrovm` + `CreateMicrovmAuthToken` + `curl /exec` spike.

2. **Phase 1 — remote shell**
   - Add `cmd/vmagent` and `/v1/exec`.
   - Add `workspaceruntime.RemoteShell` implementing `exec.Shell`.
   - Validate with a minimal image.

3. **Phase 2 — Lambda environment provider**
   - Add `internal/environments` with a `lambda` provider.
   - Implement image create/version, run/suspend/resume/terminate.
   - Extend runtime `Ref` and SQLite with `microvm_id`/endpoint.
   - Implement repo tarball down/up via `/bootstrap` and `/tar`.
   - Add durable `create_environment` / `sync_environment` job kinds.

4. **Phase 3 — preparation in the VM**
   - Run setup/verify through `/v1/exec`.
   - Persist output and redaction.

5. **Phase 4 — execution rooms**
   - Add execution-room worker and single-shell agent tool.
   - Add per-workspace mutex, run cancel, and timeout.
   - Reconnect session chat to real `/v1/exec` results.

6. **Phase 5 — reliability hardening**
   - Startup recovery and VM reconciliation.
   - Auto-suspend/resume with idle policy.
   - Mock the AWS client in tests using injected HTTP transports.
   - Run Go formatting, tests, race checks, vet, and CGO-disabled build.

7. **Phase 6 — Firecracker provider**
   - Implement after Phase 5 is stable and only if cost/scale/latency require it.

## 15. Simpler strategy summary

Keep the controller one Go process with SQLite. Treat AWS Lambda MicroVMs as the
first execution substrate behind the existing `Runtime` and `exec.Shell`
contracts. Put a tiny `vmagent` inside the VM. Keep Git authority in the
controller and advance code through the AWS data-plane HTTPS endpoint. Keep the
VM dumb and the durable state machine in SQLite. Later, Firecracker becomes a
provider swap rather than a rewrite.

## Sources

This plan was researched using `parallel-cli` web search/fetch plus direct reads
of official documentation and SDK source. The local notes under `aws/` were also
reviewed as input.

### Official AWS Lambda MicroVMs documentation

- AWS Lambda MicroVMs guide:
  https://docs.aws.amazon.com/lambda/latest/dg/lambda-microvms-guide.html
- Create your first Lambda MicroVM:
  https://docs.aws.amazon.com/lambda/latest/dg/microvms-getting-started.html
- MicroVM images:
  https://docs.aws.amazon.com/lambda/latest/dg/microvms-images.html
- Running and using MicroVMs:
  https://docs.aws.amazon.com/lambda/latest/dg/microvms-launching.html
- Networking:
  https://docs.aws.amazon.com/lambda/latest/dg/microvms-networking.html
- Security and permissions:
  https://docs.aws.amazon.com/lambda/latest/dg/microvms-security.html
- Monitoring:
  https://docs.aws.amazon.com/lambda/latest/dg/microvms-monitoring.html
- Best practices:
  https://docs.aws.amazon.com/lambda/latest/dg/microvms-best-practices.html
- Troubleshooting:
  https://docs.aws.amazon.com/lambda/latest/dg/microvms-troubleshooting.html
- Lambda MicroVMs API reference:
  https://docs.aws.amazon.com/lambda/latest/microvm-api/API_Operations.html

### AWS Agent Toolkit for AWS — Lambda MicroVMs skill

- Skill overview:
  https://github.com/aws/agent-toolkit-for-aws/blob/main/skills/specialized-skills/serverless-skills/aws-lambda-microvms/SKILL.md
- Getting started:
  https://github.com/aws/agent-toolkit-for-aws/blob/main/skills/specialized-skills/serverless-skills/aws-lambda-microvms/references/getting-started.md
- Lifecycle model:
  https://github.com/aws/agent-toolkit-for-aws/blob/main/skills/specialized-skills/serverless-skills/aws-lambda-microvms/references/lifecycle-model.md
- Networking:
  https://github.com/aws/agent-toolkit-for-aws/blob/main/skills/specialized-skills/serverless-skills/aws-lambda-microvms/references/networking.md
- IAM and security:
  https://github.com/aws/agent-toolkit-for-aws/blob/main/skills/specialized-skills/serverless-skills/aws-lambda-microvms/references/iam-and-security.md
- Snapshots and uniqueness:
  https://github.com/aws/agent-toolkit-for-aws/blob/main/skills/specialized-skills/serverless-skills/aws-lambda-microvms/references/snapshots-and-uniqueness.md
- Troubleshooting:
  https://github.com/aws/agent-toolkit-for-aws/blob/main/skills/specialized-skills/serverless-skills/aws-lambda-microvms/references/troubleshooting.md

### AWS SDK for Go v2

- `service/lambdamicrovms` package:
  https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/lambdamicrovms
- `service/lambdamicrovms` source:
  https://github.com/aws/aws-sdk-go-v2/tree/main/service/lambdamicrovms
- `service/lambdacore` source (needed for custom network connectors):
  https://github.com/aws/aws-sdk-go-v2/tree/main/service/lambdacore

### Verified community/API research

- AWS Lambda MicroVMs API facts, quotas, rate limits, and Gotchas:
  https://github.com/issakakar/aws-microvm-sandbox/blob/main/docs/API-FACTS.md
- AWS Compute Blog — Secure code execution for AI agents with AWS Lambda MicroVMs:
  https://aws.amazon.com/blogs/compute/secure-code-execution-for-ai-agents-with-aws-lambda-microvms/
- Lambda MicroVMs architecture guide:
  https://adamontherun.github.io/agent-sandboxes/chapters/ch03.html
- Lambda MicroVMs pricing deep dive:
  https://murraycole.com/posts/aws-lambda-microvms-pricing-deep-dive
- Firecracker background:
  https://northflank.com/blog/what-is-aws-firecracker
