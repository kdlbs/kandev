---
status: current
system: agents
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
---

# Managed npm runtime recovery system design

## Purpose and boundaries

The lifecycle manager owns task-session recovery policy. The host utility
manager owns host capability-probe recovery policy. Each manager reconstructs
commands only from trusted managed-runtime metadata. The colocated `agentctl`
instance owns cache discovery and exact cache repair.

This split applies to host utility probes and to `standalone`, `docker`, and
`ssh` runtimes. The design does not add executor-specific shell commands to the
Kandev backend.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001` | [Recovery flow](#recovery-flow) |
| `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002` | [Executor-local cache contract](#executor-local-cache-contract) |

## Components and responsibilities

- `runtime/lifecycle.Manager` classifies bounded startup evidence and limits recovery to one retry.
- `agent/hostutility.Manager` classifies a structured probe failure, repairs through its warm agentctl instance, and limits each probe operation to one retry.
- The host utility instance admits ordinary probes and one-shot prompts concurrently, but cache repair and its retry take exclusive admission so no process can use the execution tree while it is replaced.
- `backendapp` starts the host utility manager once, after temporary-artifact ownership is available, and runs profile and utility reconciliation after that bootstrap.
- `runtime/agentctl.Client` calls the authenticated cache-repair endpoint on the session-scoped `agentctl` instance.
- `agentctl/server/api.Server` validates the request and coordinates the local repair operation.
- `common/npmresolution` owns the strict shared npm diagnostic matcher used by both runtime lifecycle and agentctl probe classification.
- `agentctl/server/utility.ACPInferenceExecutor` validates the exact managed package in the trusted command, classifies captured probe stderr, and returns only a stable failure code to the backend.
- `agentctl/server/process.Manager` runs `npm config get cache` with the configured agent environment.
- `agent/managedruntime` validates the package specification and removes one deterministic `_npx` tree.

## Probe failure contract

`ProbeResponse` can carry a stable managed-runtime npm-resolution failure code.
Agentctl sets it only when bounded stderr contains npm `ETARGET` and a missing
top-level package specification that exactly matches the trusted probe command.
Raw stderr, npm log paths, cache paths, and registry URLs remain in agentctl
diagnostic logs and do not cross the probe API.

The host utility manager accepts this code only for an agent that implements
`ManagedNPMRuntimeAgent`. It derives the package specification and
online-preferred replacement command from that agent's managed-runtime spec and
effective version. No response field supplies executable command data.

## Executor-local cache contract

The backend sends the trusted exact package specification to the authenticated
session-scoped `agentctl` API. The request does not contain a cache path,
registry URL, shell command, or package data from stderr.

The `agentctl` process resolves the cache root with its current agent
environment. This environment includes `NPM_CONFIG_CACHE`, `HOME`, npm
configuration, and profile values that also affect the failed child process.
Host utility repair requests carry the same runtime environment overrides and
strip list as the failed probe. Agentctl applies them while resolving npm's
cache, so the repair and retry target the same effective cache.

The repair operation uses `managedruntime.RemoveNpxExecutionTree`. This helper
derives the `_npx` key from the trusted package specification. Its descriptor
walk rejects symbolic links and path replacement races.

The endpoint requires the existing agentctl bearer token and instance identity.
It accepts one exact stable package specification. It returns no host path or
raw npm output.

## Recovery flow

1. Kandev starts the exact managed runtime with `--prefer-offline`.
2. The process exits before ACP initialization.
3. The lifecycle manager reads bounded sanitized stderr from `agentctl`.
4. Recovery requires npm `ETARGET` and a matching missing `package@version` message.
5. The lifecycle manager stops the failed child process.
6. The same `agentctl` instance resolves its npm cache and removes one execution tree.
7. Kandev changes only `--prefer-offline` to `--prefer-online`.
8. Kandev starts the replacement child and initializes the original ACP session.

The startup generation rejects delayed events from the first child. The
existing cancellation and shutdown gates remain authoritative during repair.

For a host capability probe, the equivalent flow is:

1. Backend startup creates one host utility lifecycle and the manager runs the exact managed runtime with `--prefer-offline`.
2. Agentctl reports the stable managed-runtime npm-resolution failure code.
3. The host utility manager asks the same warm agentctl instance to repair the exact execution tree.
4. The host utility manager rebuilds the same effective package version with `--prefer-online` and retries once.
5. A successful retry becomes the live capability or model-configuration catalogue. A repair, retry-preparation, or final probe failure becomes the published failed status.

Normal host utility probes and prompts finish before an exclusive cache repair
can start. New operations wait until repair and its online retry complete.
The probe retry does not run profile reconciliation between attempts. Persisted
profile model, fallback model, mode, enabled state, and active runtime version
remain unchanged on both success and failure.

## Failure behavior

If cache discovery or repair fails, Kandev emits `agent_runtime`. If the
second npm attempt fails, Kandev emits `managed_runtime_npm_resolution`.

Both errors contain bounded sanitized details. The UI keeps the existing
single **Retry runtime** action. Kandev does not change the active version.

Unsupported runtime types do not call the repair endpoint. Native commands,
passthrough commands, unrelated npm errors, and repeated failures remain on the
normal terminal error path.

A host capability probe that cannot repair or fails its online retry publishes
the final failure normally. Kandev does not hide a runtime that still cannot
start, change its version, or substitute a stale capability catalogue from a
different runtime generation.

## Observability

Structured recovery logs include the recovery scope, agent ID, attempt, outcome,
and, for task sessions, the execution ID, runtime type, and startup generation.
They do not include cache paths, registry URLs, or raw stderr.

Existing failure metadata stores the stable failure code and sanitized details.
No database migration is necessary.

## Related decisions

- [Validate and persist managed runtime version selection](../../../decisions/2026-08-12-validated-managed-runtime-version-selection.md)
- [Run cache repair where npm runs](../../../decisions/2026-08-24-agentctl-local-managed-runtime-cache-repair.md)
- [Recover host capability probes before publishing failure](../../../decisions/2026-09-07-host-utility-managed-runtime-recovery.md)
