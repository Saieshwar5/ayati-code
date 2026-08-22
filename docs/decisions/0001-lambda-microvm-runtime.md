# ADR 0001: AWS Lambda MicroVMs as the first production runtime

Status: accepted

## Context

Perpetual previously executed workspace setup and agent commands in a bounded
local shell on the controller host. Docker was removed, and the local shell was
kept as an intermediate mode while isolated compute was designed. The product
now needs isolated, stateful execution environments for a coding agent.

## Decision

Use AWS Lambda MicroVMs as the first production execution substrate behind the
existing `internal/workspaceruntime.Runtime` and `internal/exec.Shell`
contracts.

- Local remains the development/compatibility runtime.
- AWS control-plane calls live in a Lambda MicroVMs provider.
- Workspace code is synced into a MicroVM by tarball, not via GitHub
  credentials inside the VM.
- Agent shell calls are HTTP-backed `/v1/exec` requests to an in-VM `vmagent`.
- Git, GitHub credentials, SQLite state, scheduling, commits, pushes, and pull
  requests remain controller-owned.

## Rationale

- Firecracker VM isolation with managed service operations.
- Snapshot-based start and suspend/resume with zero compute while suspended.
- Dedicated, authenticated HTTPS data plane.
- Matching Go SDK v2 support.
- Keeps Perpetual control plane small; no host fleet, worker cluster, or custom
  virtualization layer required initially.

## Rejected alternatives

- Self-managed Firecracker initially: requires owning hosts, guest images, TAP
  networking, snapshots, and lifecycle operations. Revisit only when cost,
  scale, or policy require it.
- ECS/Fargate or EC2 per workspace: heavier lifecycle and weaker sandbox
  isolation model.
- Temporal/Kafka/Postgres orchestration: unnecessary external dependencies for
  the current durable SQLite job model.

## Consequences

- The controller must manage AWS IAM, S3 artifacts, image versions, auth token
  refresh, suspend/resume, and rate limits.
- Workspaces must support both local and cloud runtimes during the transition.
- Cloud mode must fail closed when AWS configuration is invalid.
- A common `vmagent` contract must stay provider-neutral so a future
  Firecracker provider is a swap behind `Runtime`.

## Migration

1. Implement `cmd/vmagent` and the remote `exec.Shell`.
2. Implement the Lambda MicroVMs provider.
3. Move preparation sync into the MicroVM.
4. Wire execution rooms to the selected runtime.
5. Re-evaluate self-managed Firecracker only after the Lambda path is stable in
   operation and cost.
