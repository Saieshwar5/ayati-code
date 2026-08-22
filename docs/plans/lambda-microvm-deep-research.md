# AWS Lambda MicroVMs — Deep Technical Research

Status: research notes; verified against the AWS Lambda MicroVMs Developer
Guide, the AWS Agent Toolkit for AWS, the AWS SDK for Go v2, and the official
MicroVM API reference.

## 1. What Lambda MicroVMs are

AWS Lambda MicroVMs is a serverless, stateful sandbox service. Each MicroVM:

- runs as a container inside a Firecracker microVM, so it has VM-level isolation;
- runs Amazon Linux 2023 as the guest OS;
- boots by resuming a **memory + disk snapshot** taken at image build time;
- supports a stateful lifetime up to **8 hours**;
- gets a **dedicated TLS HTTPS endpoint** with a short-lived auth token;
- can be **suspended and resumed** with memory, disk, and running processes intact;
- is billed by the second while running, zero compute while suspended, and
  snapshot storage while suspended.

This is the right first substrate for Perpetual because it solves VM isolation,
fast resume, networking, authentication, suspend/resume, and snapshot lifecycle
without Perpetual operating a VM fleet.

## 2. Two-resource model

The service has two resources:

1. **MicrovmImage**

   A versioned image produced from:

   - an S3 ZIP containing a `Dockerfile` at the ZIP root plus application files;
   - a Lambda-managed base image ARN, for example
     `arn:aws:lambda:<region>:aws:microvm-image:al2023-1`;
   - a build IAM role.

2. **Microvm**

   A concrete running instance returned by `RunMicrovm`. A Microvm has:

   - `microvmId`;
   - a unique HTTPS `endpoint`;
   - router/ingress and egress network connectors;
   - an idle policy;
   - a maximum lifetime.

## 3. Important facts that must drive the design

- Container base image and Lambda base image are different. The Dockerfile `FROM`
  defines the application container; `baseImageArn` defines the AL2023 guest OS
  environment.
- The application is snapshot while it is already running. Do **not** generate
  per-VM IDs, secrets, or PRNG secrets at build time.
- The default data-plane port inside the VM is **8080**.
- Application lifecycle hooks must listen on **0.0.0.0:<hooks.port>**; port
  **9000** is the practical convention.
- Inbound traffic to the endpoint starts only after the `/run` hook returns 200.
- Idle is measured by inbound traffic through the service proxy. A VM that does
  outbound-only background work without inbound requests is counted as idle.
- Auth tokens are JWE values, scoped to MicroVM ID + ports + expiration.
- Auth token maximum TTL is **60 minutes**.
- `RunMicrovm` has a hard lifetime cap of **28,800 seconds (8 hours)**.
- Outbound internet is available by default. VPC egress needs a separate
  `lambda-core` network connector.

## 4. Resource sizes

Official sizing table:

| Baseline | Peak | Max disk |
|---|---|---|
| 0.5 GB memory, 0.25 vCPU | 2 GB, 1 vCPU | 8 GB |
| 1 GB memory, 0.5 vCPU | 4 GB, 2 vCPU | 8 GB |
| 2 GB memory, 1 vCPU (default) | 8 GB, 4 vCPU | 8 GB |
| 4 GB memory, 2 vCPU | 16 GB, 8 vCPU | 16 GB |
| 8 GB memory, 4 vCPU | 32 GB, 16 vCPU | 32 GB |

Bandwidth through the proxy scales with size:

| Baseline | Max bandwidth |
|---|---|
| 0.5 GB | 1 MB/s (8 Mbps) |
| 1 GB | 2 MB/s (16 Mbps) |
| 2 GB | 4 MB/s (32 Mbps) |
| 4 GB | 8 MB/s (64 Mbps) |
| 8 GB | 16 MB/s (128 Mbps) |

For a coding workspace, start with **2 GB / 1 vCPU**. Do not jump to 8 GB;
snapshot size and latency both increase.

## 5. Availability

The research sources list **us-east-1, us-east-2, us-west-2, eu-west-1, and
ap-northeast-1** as available at the time of writing. The reliable check is:

```bash
aws lambda-microvms list-managed-microvm-images --region <region>
```

S3 artifacts, images, and connectors must all be in the same region.

## 6. Image build process

Prerequisites:

1. S3 bucket in the target region.
2. Build IAM role trusted by `lambda.amazonaws.com`.
3. Optional execution IAM role.

Build input:

- a ZIP containing `Dockerfile` at the root;
- `baseImageArn`;
- `buildRoleArn`.

Build flow:

1. Lambda retrieves the ZIP from S3.
2. Starts a fresh MicroVM from the managed base image.
3. Runs the application Dockerfile.
4. Starts the application using `CMD`/`ENTRYPOINT`.
5. Calls `/ready` if enabled.
6. Captures the disk + memory snapshot.
7. Optionally runs `/validate`.

Common build error codes:

`S3_ACCESS_DENIED`, `S3_NO_SUCH_KEY`, `S3_NO_SUCH_BUCKET`,
`S3_INVALID_OBJECT`, `S3_CROSS_REGION_ACCESS_DENIED`,
`ARCHIVE_DOCKERFILE_NOT_FOUND`, `ARCHIVE_INVALID`, `CONTAINER_BUILD_FAILED`,
`DISK_STORAGE_FULL`, `INTERNAL_PLATFORM_ERROR`.

## 7. Image and version state machines

Image state:

`CREATING`, `CREATED`, `CREATION_FAILED`, `UPDATING`, `UPDATED`,
`UPDATE_FAILED`, `DELETING`, `DELETED`, `DELETION_FAILED`

Version build state:

`PENDING`, `IN_PROGRESS`, `SUCCESSFUL`, `FAILED`

Version activation:

`ACTIVE`, `INACTIVE`

A MicroVM can run only when the image is `CREATED`/`UPDATED`, the version is
`SUCCESSFUL`, and the version is `ACTIVE`.

Every update creates a new version. Old versions continue to cost snapshot/image
storage until deleted.

## 8. MicroVM state machine

```
PENDING -> RUNNING -> SUSPENDING -> SUSPENDED -> TERMINATING -> TERMINATED
```

Triggers:

- `RunMicrovm` -> `PENDING` -> `RUNNING`
- idle time exceeds `maxIdleDurationSeconds` -> suspend
- explicit `SuspendMicrovm` -> suspend
- `autoResumeEnabled=true` + inbound request -> resume
- explicit `ResumeMicrovm` -> resume
- `suspendedDurationSeconds` elapsed -> terminate
- `maximumDurationInSeconds` reached -> terminate
- explicit `TerminateMicrovm` -> terminate

Idle policy fields are all required when the policy is supplied:

- `maxIdleDurationSeconds` must be `>= 60`.
- `suspendedDurationSeconds` controls time-to-terminate while suspended.
- `autoResumeEnabled` controls transparent resume on ingress.

## 9. Lifecycle hook contract

All hooks are optional HTTP POST endpoints.

Build-time hooks:

| Hook | Path | Purpose | Timeout |
|---|---|---|---|
| `/ready` | `/aws/lambda-microvms/runtime/v1/ready` | confirm app initialized before snapshot | 1-3600s |
| `/validate` | `/aws/lambda-microvms/runtime/v1/validate` | smoke-test snapshot and prefetch | 1-3600s |

Runtime hooks:

| Hook | Path | Purpose | Timeout |
|---|---|---|---|
| `/run` | `/aws/lambda-microvms/runtime/v1/run` | per-VM initialization after snapshot resume | 1-60s |
| `/resume` | `/aws/lambda-microvms/runtime/v1/resume` | refresh connections after suspend | 1-60s |
| `/suspend` | `/aws/lambda-microvms/runtime/v1/suspend` | flush before suspend | 1-60s |
| `/terminate` | `/aws/lambda-microvms/runtime/v1/terminate` | final flush | 1-60s |

Hook rules:

- bind to `0.0.0.0`, not loopback;
- `/ready` and `/validate` return 200 or quick 503; never hold a request open;
- `/run` receives `{ "microvmId": "...", "runHookPayload": "..." }`;
- `/run` must return quickly; it is signal/init, not a long task;
- `/run` returning 200 starts external traffic;
- hooks must be idempotent.

## 10. Networking

### Ingress

- Each MicroVM has a unique endpoint:
  `https://<microvm-id>.lambda-microvm.<region>.on.aws`.
- Default target port: 8080.
- Override target port with header `X-aws-proxy-port: <port>`.
- Auth header: `X-aws-proxy-auth: <token>`.
- Token can allow one port, a range, or all ports:

```text
{"port":8080}
{"range":{"startPort":8080,"endPort":8081}}
{"allPorts":{}}
```

- WebSocket auth/port travel through subprotocols:

```text
lambda-microvms
lambda-microvms.authentication.<token>
lambda-microvms.port.<port>
```

- Supported protocols: HTTP/1.1, HTTP/2, gRPC, WebSocket, SSE.
- The `x-aws-proxy-*` namespace is reserved and stripped before the app sees it.

### Egress

- Public internet egress is available by default.
- For VPC egress, create a `lambda-core` network connector, wait for `ACTIVE`,
  and pass its ARN in `egressNetworkConnectors`.
- Connectors are bound at run time and cannot be changed on suspend/resume.
- For internet and VPC access simultaneously, use a NAT gateway in the VPC.

## 11. IAM and AWS credentials

### Credentials the Perpetual controller needs

The controller authenticates to AWS through the normal AWS Go SDK credential
chain:

- `AWS_ACCESS_KEY_ID` + `AWS_SECRET_ACCESS_KEY` (+ optional
  `AWS_SESSION_TOKEN` for temporary credentials);
- shared credentials file `~/.aws/credentials`;
- SSO / Web Identity / EC2 instance role / ECS task role, when applicable.

Never put AWS credentials inside the microVM shell environment. The controller
uses them only for AWS control-plane calls.

### Controller policy

The controller needs at minimum:

- `lambda:CreateMicrovmImage`
- `lambda:UpdateMicrovmImage`
- `lambda:DeleteMicrovmImage`
- `lambda:DeleteMicrovmImageVersion`
- `lambda:GetMicrovmImage`
- `lambda:ListMicrovmImages`
- `lambda:ListManagedMicrovmImages`
- `lambda:ListManagedMicrovmImageVersions`
- `lambda:RunMicrovm`
- `lambda:GetMicrovm`
- `lambda:ListMicrovms`
- `lambda:SuspendMicrovm`
- `lambda:ResumeMicrovm`
- `lambda:TerminateMicrovm`
- `lambda:CreateMicrovmAuthToken`
- `lambda:PassNetworkConnector`
- `iam:PassRole` on the execution role, if used
- `s3:PutObject` and `s3:GetObject` on the image-artifact bucket

The IAM action prefix for MicroVM operations is **`lambda:`**.

### Build role

Trust policy principal: `lambda.amazonaws.com`.

Minimum permissions:

- `s3:GetObject` on the artifact key;
- `logs:CreateLogGroup`, `logs:CreateLogStream`, `logs:PutLogEvents`;
- ECR read permissions if the Dockerfile `FROM` uses private ECR.

Example trust-policy condition to prevent confused deputy:

```json
{
  "Effect": "Allow",
  "Principal": { "Service": "lambda.amazonaws.com" },
  "Action": ["sts:AssumeRole", "sts:TagSession"],
  "Condition": {
    "StringEquals": { "aws:SourceAccount": "<account-id>" },
    "ArnLike": { "aws:SourceArn": "arn:aws:lambda:<region>:<account-id>:microvm-image:*" }
  }
}
```

### Execution role

- Optional, but required for CloudWatch runtime logs.
- Optional for IMDSv2 credentials inside the VM.
- Runtime hooks run under the execution role.
- Exposed to the guest at `http://169.254.169.254/latest/meta-data/iam/security-credentials/execution_role`.

## 12. Shell access (debug only)

Attach the AWS-managed `SHELL_INGRESS` connector at run time:

```text
arn:aws:lambda:<region>:aws:network-connector:aws-network-connector:SHELL_INGRESS
```

Then create a **separate** shell token with
`CreateMicrovmShellAuthToken` and connect over WebSocket to the shell channel
(typically port 8022). This is useful for debugging, but Perpetual's own
agent should use its in-VM HTTP executor, not shell-ingress for normal work.

## 13. Pricing

Unofficial but consistent public estimates for us-east-1 ARM:

- vCPU: `$0.0000276944`/vCPU-second ~ `$0.0997`/vCPU-hour
- memory: `$0.0000036667`/GB-second ~ `$0.0132`/GB-hour
- snapshot write/read and storage also apply.

At baseline 2 GB / 1 vCPU:

- running rate is about `$0.126`/hour before burst;
- suspended rate is **zero compute**; only snapshot storage (`$0.08`/GB-month).

Cost controls:

- always set `maximumDurationInSeconds`;
- use an idle policy;
- terminate unused VMs promptly;
- delete old image versions.

## 14. Go SDK v2 dependencies

Add to `go.mod`:

```text
github.com/aws/aws-sdk-go-v2
github.com/aws/aws-sdk-go-v2/config
github.com/aws/aws-sdk-go-v2/service/lambdamicrovms
github.com/aws/aws-sdk-go-v2/service/lambdacore        # only for VPC connectors
github.com/aws/aws-sdk-go-v2/service/s3                # artifact upload/download
github.com/aws/aws-sdk-go-v2/service/sts               # optional identity checks
```

Install:

```bash
go get github.com/aws/aws-sdk-go-v2/config@latest
go get github.com/aws/aws-sdk-go-v2/service/lambdamicrovms@latest
go get github.com/aws/aws-sdk-go-v2/service/lambdacore@latest
go get github.com/aws/aws-sdk-go-v2/service/s3@latest
```

The SDK v2 service metadata:

- package: `github.com/aws/aws-sdk-go-v2/service/lambdamicrovms`
- service ID: `Lambda Microvms`
- API version: `2025-09-09`
- signing name: `lambda`

## 15. Go client construction

```go
import (
    "context"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    mvms "github.com/aws/aws-sdk-go-v2/service/lambdamicrovms"
    mvmtypes "github.com/aws/aws-sdk-go-v2/service/lambdamicrovms/types"
)

func newClient(ctx context.Context, region string) (*mvms.Client, error) {
    cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
    if err != nil {
        return nil, err
    }
    return mvms.NewFromConfig(cfg), nil
}
```

Run a MicroVM:

```go
out, err := client.RunMicrovm(ctx, &mvms.RunMicrovmInput{
    ImageIdentifier: aws.String(imageARN),
    ImageVersion:    aws.String("1.0"),
    IdlePolicy: &mvmtypes.IdlePolicy{
        MaxIdleDurationSeconds:    aws.Int32(900),
        SuspendedDurationSeconds:  aws.Int32(1800),
        AutoResumeEnabled:         aws.Bool(true),
    },
    MaximumDurationInSeconds: aws.Int32(8 * 60 * 60),
})
```

Create an auth token:

```go
tokenOut, err := client.CreateMicrovmAuthToken(ctx, &mvms.CreateMicrovmAuthTokenInput{
    MicrovmIdentifier: aws.String(*out.MicrovmId),
    ExpirationInMinutes: aws.Int32(30),
    AllowedPorts: []mvmtypes.PortSpecification{
        &mvmtypes.PortSpecificationMemberPort{Value: 8080},
    },
})
authHeader := tokenOut.AuthToken["X-aws-proxy-auth"]
```

Important Go SDK types:

- `types.IdlePolicy`
- `types.PortSpecification`
- `types.PortSpecificationMemberPort`
- `types.PortSpecificationMemberRange`
- `types.PortSpecificationMemberAllPorts`
- `types.Hooks`, `types.MicrovmHooks`, `types.MicrovmImageHooks`
- `types.CodeArtifactMemberUri`
- `types.Resources`
- `types.MicrovmState`, `types.MicrovmImageState`, `types.HookState`
- `types.Logging`, `types.CloudWatchLogging`, `types.LoggingMemberDisabled`

## 16. Rate limits and operation behavior

Confirmed API Facts:

- `RunMicrovm` <= 5 TPS
- `ResumeMicrovm` <= 5 TPS
- `SuspendMicrovm` <= 2 TPS
- `TerminateMicrovm` <= 10 TPS
- `CreateMicrovmAuthToken` <= 50 TPS
- `GetMicrovm` <= 100 TPS

All provider calls should use retry with exponential backoff and jitter. Do not
poll `GetMicrovm` in a tight loop; it is eventually consistent. Readiness is
best determined by actually trying the endpoint.

## 17. How the Perpetual controller should manage them

### Controller responsibilities

- owns AWS credentials and control-plane calls;
- owns GitHub credentials and Git/PR operations;
- owns SQLite durable state;
- treats the MicroVM as an isolated work directory;
- does not send GitHub credentials to the MicroVM;
- sends workspace environment values only over the authenticated VM endpoint.

### In-VM executor

Build one small `vmagent` binary that runs in every Perpetual environment image:

- listens on 8080 for controller requests;
- listens on 9000 for Lambda lifecycle hooks;
- endpoints: `/healthz`, `/v1/bootstrap`, `/v1/exec`, `/v1/tar`;
- `/v1/exec` uses the existing `exec.ShellRequest` / `exec.ShellResult` shape;
- `/run` re-generates per-VM entropy and prepares state;
- `/suspend` purges transient workspace environment values;
- `/resume` re-initializes after a controller re-bootstrap.

### Lifecycle mapping

| Controller operation | Lambda MicroVMs operation |
|---|---|
| environment Start | `RunMicrovm` |
| environment Stop | `SuspendMicrovm` |
| environment Resume | `ResumeMicrovm` |
| environment Destroy | `TerminateMicrovm` |
| shell command | `POST https://<endpoint>/v1/exec` |

### Durability pattern

1. Persist intended state in SQLite.
2. Call AWS control plane.
3. Persist returned `microvmId`, `endpoint`, `imageArn`, and `imageVersion`.
4. On controller restart, reconcile SQLite `runtime_ref` against `ListMicrovms`.
5. Recover queued/running durable jobs just like the current `workspace_jobs`.

### Repository sync

1. Controller clones with trusted host Git.
2. Controller runs/updates the MicroVM.
3. Controller streams a repo tarball to `/v1/bootstrap`.
4. Setup and agent commands run through `/v1/exec`.
5. Before review/publish, controller fetches `/v1/tar` and runs host Git diff,
   commit, push, and PR.

## 18. Security checklist

- Separate build and execution roles.
- Use `aws:SourceAccount` / `aws:SourceArn` in trust policies.
- Use the smallest allowed ports on auth tokens.
- Refresh auth tokens before the 60-minute maximum, not after failure arrives.
- Scope token validity to 15-30 minutes for long agent work.
- Keep AWS and GitHub credentials out of the VM image and VM environment.
- Use VPC egress if the VM must reach private services.
- Audit control plane with CloudTrail data events.
- Delete old image versions and unused MicroVMs.

## 19. Decision summary

Use AWS Lambda MicroVMs as the first execution substrate behind the existing
`internal/workspaceruntime.Runtime` and `internal/exec.Shell` contracts. Keep
the controller stateless in execution, stateful in SQLite, and make the in-VM
agent contract small and reusable so a future Firecracker provider is a runtime
swap rather than a product rewrite.

## Source references

Researched with `parallel-cli` search/fetch and direct reads of the following
locations.

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
- `service/lambdacore` source:
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
