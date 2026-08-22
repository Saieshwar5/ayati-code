# Perpetual cloud architecture

Status: target architecture for AWS Lambda MicroVMs execution.

This document describes the controller's cloud execution path. It extends
`docs/architecture.md` and is the implementation target for the runtime and
environment work. The local runtime remains the development and fallback
adapter.

## 1. Shape

The control plane remains one Go process with SQLite, GitHub OAuth, durable
jobs, SSE progress, and Git/PR ownership. Compute moves to isolated microVMs.

```text
Browser UI
   |
   | JSON API + SSE invalidation
   v
Controller (Go process + SQLite)
   |                          \
   | GitHub API                \ AWS control-plane API
   v                            v
GitHub repos/PRs          Lambda MicroVMs service
                                |
                                | authenticated HTTPS data plane
                                v
                          workspace MicroVM
                          (vmagent + working tree + toolchain)
```

## 2. Runtime mapping

| Controller contract | AWS Lambda MicroVMs action |
|---|---|
| `Runtime.Start` | `RunMicrovm` |
| `Runtime.Stop` | `SuspendMicrovm` |
| `Runtime.Resume` | `ResumeMicrovm` |
| `Runtime.Destroy` | `TerminateMicrovm` |
| `Runtime.OpenShell` | HTTP-backed `exec.Shell` -> `vmagent` `/v1/exec` |

A workspace records the resulting `microvmId` in `runtime_ref` and stores
`runtime_state` transitions in SQLite before and after AWS calls.

## 3. In-VM agent contract

Every Perpetual environment image contains a small `vmagent` binary.

Listeners:

- `8080` — controller data plane.
- `9000` — Lambda lifecycle hooks.

Endpoints:

- `GET /healthz` — liveness.
- `POST /v1/bootstrap` — receive workspace tarball, workdir, and setup inputs.
- `POST /v1/exec` — one bounded `/bin/sh -c` command.
- `GET /v1/tar` — stream the current working tree back to the controller.

`/v1/exec` serializes the existing `exec.ShellRequest` and `exec.ShellResult`,
so the execution-room agent sees one tool regardless of runtime.

Lifecycle hooks:

- `/run` re-initializes per-instance entropy and prepares the workspace.
- `/suspend` purges workspace environment values.
- `/resume` restores runtime state.
- `/terminate` flushes state before the VM is released.

## 4. Image build

AWS Lambda MicroVMs builds an image from an S3 ZIP containing a `Dockerfile`
plus application files. Perpetual keeps its microVM image generic:

- `vmagent`;
- Go, Node, and Python toolchains;
- no repository content and no secrets.

When useful, a second image tier can be built from an `EnvironmentSpec`
fingerprint to preinstall project dependencies. The controller stores the
resulting `imageArn` and `imageVersion` as the environment version artifact
reference.

Go SDK v2 dependencies:

```text
github.com/aws/aws-sdk-go-v2
github.com/aws/aws-sdk-go-v2/config
github.com/aws/aws-sdk-go-v2/service/lambdamicrovms
github.com/aws/aws-sdk-go-v2/service/lambdacore   # only for custom connectors
github.com/aws/aws-sdk-go-v2/service/s3           # image artifact upload
```

## 5. Repository sync

Git/GitHub credentials never enter the microVM.

1. Controller clones or opens the repository with trusted host Git.
2. Controller creates/resumes the MicroVM.
3. Controller streams a repository tarball to `/v1/bootstrap`.
4. Setup and agent commands run through `/v1/exec`.
5. Controller fetches `/v1/tar` before review/publish and runs trusted host Git
   diff, commit, push, and PR.

## 6. Durable control flow

Cloud operations use the existing durable `workspace_jobs` primitive:

- `prepare_workspace` — source acquisition, spec analysis, environment binding.
- `build_environment` — build/verify the version, or build the MicroVM image.
- `sync_environment` (future) — push working tree down or collect it up.
- `run_agent` (future) — execution-room run.

The controller persists intended state, makes the AWS call, then records the
returned provider reference. Startup recovery reconciles `runtime_ref` rows
against `ListMicrovms`.

## 7. AWS configuration and IAM

Controller AWS credentials come from the standard AWS Go SDK credential chain:
environment variables, shared credentials file, SSO/Web Identity, or EC2/ECS
instance/task role. They are never placed in the workspace shell.

The controller requires at least:

- `lambda:CreateMicrovmImage`
- `lambda:UpdateMicrovmImage`
- `lambda:GetMicrovmImage`
- `lambda:ListMicrovmImages`
- `lambda:ListManagedMicrovmImages`
- `lambda:RunMicrovm`
- `lambda:GetMicrovm`
- `lambda:ListMicrovms`
- `lambda:SuspendMicrovm`
- `lambda:ResumeMicrovm`
- `lambda:TerminateMicrovm`
- `lambda:CreateMicrovmAuthToken`
- `lambda:PassNetworkConnector`
- `s3:PutObject` and `s3:GetObject` on the artifact bucket
- `iam:PassRole` where an execution role is used

Build and execution roles are separate and trust `lambda.amazonaws.com`.
Build role reads S3 and CloudWatch logs; execution role enables runtime logs
and any in-VM AWS SDK access through IMDSv2.

## 8. Provider invariants

The Lambda MicroVMs provider must:

- refresh data-plane auth tokens before the 60-minute maximum;
- use exponential backoff and jitter for AWS rate limits;
- never poll `GetMicrovm` in a hot loop; readiness is best determined by
  connecting;
- set `maximumDurationInSeconds` and an idle policy on every instance;
- keep image artifacts and the image in the same region;
- delete old image versions to avoid persistent storage cost;
- clean up drifting instances during startup reconciliation.

## 9. Security boundaries

- AWS and GitHub credentials are controller-only.
- The microVM receives repo data by tarball and workspace environment values
  only through the authenticated data plane.
- `vmagent` purges workspace environment values on `/suspend`.
- Git commits and pull requests are controller-owned, never agent-owned.
- Workspace/agent rows remain `user_id` scoped.
- One active execution room per workspace avoids concurrent working-tree
  mutation.

## 10. Local development

The controller keeps the local runtime behind the same `Runtime` contract so
tests, offline development, and small personal workspaces continue to work
without AWS. Cloud mode is selected through configuration and must fail
explicitly when credentials/config are missing instead of silently returning to
local execution.

## Source references

The detailed service notes and links are in
`docs/plans/lambda-microvm-deep-research.md`.
