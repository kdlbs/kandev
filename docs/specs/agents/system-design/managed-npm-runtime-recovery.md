---
status: draft
system: agents
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
---

# Managed npm runtime recovery system design

## Purpose and boundaries

The lifecycle manager owns recovery policy and trusted command reconstruction.
The colocated `agentctl` instance owns cache discovery and exact cache repair.

This split applies to `standalone`, `docker`, and `ssh` runtimes. The design
does not add executor-specific shell commands to the Kandev backend.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001` | [Recovery flow](#recovery-flow) |
| `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002` | [Executor-local cache contract](#executor-local-cache-contract) |

## Components and responsibilities

- `runtime/lifecycle.Manager` classifies bounded startup evidence and limits recovery to one retry.
- `runtime/agentctl.Client` calls the authenticated cache-repair endpoint on the session-scoped `agentctl` instance.
- `agentctl/server/api.Server` validates the request and coordinates the local repair operation.
- `agentctl/server/process.Manager` runs `npm config get cache` with the configured agent environment.
- `agent/managedruntime` validates the package specification and removes one deterministic `_npx` tree.

## Executor-local cache contract

The backend sends the trusted exact package specification to the authenticated
session-scoped `agentctl` API. The request does not contain a cache path,
registry URL, shell command, or package data from stderr.

The `agentctl` process resolves the cache root with its current agent
environment. This environment includes `NPM_CONFIG_CACHE`, `HOME`, npm
configuration, and profile values that also affect the failed child process.

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

## Failure behavior

If cache discovery or repair fails, Kandev emits `agent_runtime`. If the
second npm attempt fails, Kandev emits `managed_runtime_npm_resolution`.

Both errors contain bounded sanitized details. The UI keeps the existing
single **Retry runtime** action. Kandev does not change the active version.

Unsupported runtime types do not call the repair endpoint. Native commands,
passthrough commands, unrelated npm errors, and repeated failures remain on the
normal terminal error path.

## Observability

Structured recovery logs include the execution ID, agent ID, runtime type, and
startup generation. They do not include cache paths, registry URLs, or raw
stderr.

Existing failure metadata stores the stable failure code and sanitized details.
No database migration is necessary.

## Related decisions

- [Validate and persist managed runtime version selection](../../../decisions/2026-08-12-validated-managed-runtime-version-selection.md)
- [Run cache repair where npm runs](../../../decisions/2026-08-24-agentctl-local-managed-runtime-cache-repair.md)
