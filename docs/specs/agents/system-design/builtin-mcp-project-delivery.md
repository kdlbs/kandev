---
status: draft
system: agents
created: 2026-11-05
requirements:
  - REQ-AGENTS-BUILTIN-MCP-002
  - REQ-AGENTS-BUILTIN-MCP-003
  - REQ-AGENTS-BUILTIN-MCP-004
  - REQ-AGENTS-MCP-PREP-005
owners:
  - kandev
---

# Built-in MCP delivery through project configuration

## Ownership and requirement mapping

Agents owns package delivery and provider admission. This design complements
[package binding](builtin-mcp-management.md) and
[project-file ownership](agent-mcp-preparation.md#project-local-file-ownership-and-execution-generations).
It restores the Pi route established by
[ADR 0020](../../../decisions/0020-pi-project-mcp-config-injection.md).
Platform retains attachment observation ownership.

| Requirement | Design section |
| --- | --- |
| REQ-AGENTS-BUILTIN-MCP-002 | Admission and authority |
| REQ-AGENTS-BUILTIN-MCP-003 | Prepared delivery contract; lifecycle and recovery |
| REQ-AGENTS-BUILTIN-MCP-004 | Attachment evidence |
| REQ-AGENTS-MCP-PREP-005 | Materialization and compatibility |

## Confirmed fault

`acp.requireBuiltinMCPDelivery` counts only entries that survive ACP transport
filtering. Pi declares `RuntimeConfig.ProjectMCPStrategy`, and lifecycle writes
`.pi/mcp.json` before process start. Pi's lack of ACP HTTP/SSE support therefore
causes rejection even after valid project materialization.

The guard runs in `newSession`, `LoadSession`, and `ResumeSession`.
`ResetSession` reaches the same new-session path. A temporary Go overlay
reproduced rejection before any provider session RPC in all four cases.

## Prepared delivery contract

The authenticated `POST /api/v1/agent/configure` request carries a typed,
optional `ProjectMCPDelivery` record. No public settings, provider JSON, task
metadata, or environment variable can supply this record.

The record contains a version, strategy identity, canonical workspace, task ID,
execution/instance ID, canonical package-binding digest, and exact package IDs.
It contains no endpoint, credential, header, environment value, or file content.
Use the existing binding canonicalization in `internal/mcp/profile` for digest
comparison. Do not create a second package resolver.

Lifecycle creates this record only after the selected project-file writer
successfully materializes the prepared operation. Local placement uses the
backend writer and is skipped when no local port is available; remote placement
uses the helper project-file writer and the helper-provided MCP URL. The
operation must contain an output artifact and every package that the record
names. Unsupported strategies, empty artifacts, mismatched helper responses, or
failed writes cannot produce delivery evidence.

Keep the record private to `AgentExecution` until the common
`configureAndStartAgentWithPreparedArgs` handoff. Configure must copy it by value
and send it with the command for that execution. This avoids new per-executor
launch fields and protects promotion, ordinary launch, and remote launches
through the same boundary.

The configure handler requires `authenticatedControlContextKey` for an explicit
record, as it already does for a launch wrapper. An unauthenticated caller
cannot supply delivery evidence even if ordinary configuration is permitted.
Agentctl validates the record against its trusted task, instance, working
directory, and bound profile before accepting configuration. It serializes
acceptance with `process.Manager` configuration/start ownership. A missing or
changed record clears prior delivery evidence. Invalid explicit evidence fails
configuration without replacing accepted state. A successful new configuration
cannot inherit a prior process's evidence by omission.

Use the helper instance identity from the pinned client. Host execution IDs
and recovered helper IDs are distinct contracts. Do not assume equality.
The lifecycle handoff must hold the current startup attempt's ownership while
it snapshots and sends the record. Superseded preparation cannot configure a
successor process.

The accepted record authorizes exactly one process generation. The process
manager consumes pending evidence while building that generation's adapter;
the adapter retains the immutable copy for same-process load, resume, and
reset. A replacement process must receive fresh successful preparation and a
fresh authenticated configure, even on the same helper.

The process manager forwards a cloned validated record through
`adapter.Config.ToSharedConfig` into `shared.Config`. The resulting record
belongs to that adapter/process generation. Session request payloads cannot
replace it. The implementation must retain existing argument, wrapper, and
environment semantics while extending configuration.

Advertise a versioned `project-mcp-delivery-v1` helper capability. The backend
requires it before configuring a package-mode launch that depends on project
delivery. A helper without support produces upgrade guidance before process
start. Direct ACP and retained-process adoption do not require this capability.

## Implementation

`agentctl/types.ProjectMCPDelivery` owns the wire record and its structural
validation. `mcpconfig.ProjectDeliveryStrategyKey` admits only the Pi
producer today. `lifecycle.projectMCPDeliveryEvidence` builds the record from
the pinned helper instance and the successful write; the common configure
handoff validates it against the current client and negotiates the capability.
`process.validateProjectMCPDelivery` re-derives the bound digest from the
helper's own configuration, and `acp.requireBuiltinMCPDelivery` applies the
per-package admission rule to new, load, resume, and reset. Project-delivered
entries publish `project_config_materialized` delivery evidence once per
package identity.

## Admission and authority

For each requested host-injected package, session admission requires either:

1. An exact matching package entry accepted by normal ACP transport filtering.
2. An exact matching package in the current validated project-delivery record.

This is a universal condition over selected packages. Core delivery cannot
authorize a missing optional package. A mixed record with Core present and
Diagnostics absent must fail before the provider session RPC.

Project delivery never adds an entry to the ACP wire array. Pi continues to
advertise its actual capabilities. Third-party filtering, HTTP preference,
SSE fallback, reserved identities, bound routes, and dispatch guards retain
their existing behavior. No package can become selected through this record.

Apply the same admission helper to new, load, resume, and reset. Preserve native
session identity and existing restore fallback rules. A capability assumption
such as `AssumeMcpHttp` is not a substitute for successful project preparation.

## Materialization and compatibility

The first supported producer is `mcpconfig.PiStrategy`. It retains project-only
configuration, eager entries, transport spelling, user-file merge behavior, and
the existing ownership fence. No user-level Pi file is introduced.

Local and worktree executions keep the backend file writer. Executor runtimes
with a separate target filesystem use the authenticated helper project-file
contract:

- `GET /api/v1/mcp/project` returns the helper's own workspace path, loopback
  MCP URL, instance/session identity, and preparation generation. The backend
  never guesses a container, Pod, SSH, or plugin-remote path.
- `POST /api/v1/mcp/project/prepare` materializes generated files inside
  `os.Root` containment with the existing merge and ownership registry rules,
  then verifies the requested entries by reading them back and returns
  fingerprints.
- `POST /api/v1/mcp/project/cleanup` releases that execution's claims.

Delivery evidence is emitted only after the helper confirms its own identity,
workspace, endpoint, generation, and non-empty read-back evidence. The exact
package URLs are built from the helper's loopback MCP URL, so the agent reaches
the helper in its own namespace. SSH, Docker, Kubernetes, Sprites, and plugin
remote runtimes share this path.

Cursor also declares `ProjectMCPStrategy`. Keep its current ACP behavior and
native approval flow. This repair does not authorize Cursor project delivery
through the new record. A later producer needs explicit strategy validation and
its own wire and materialization evidence. Merely declaring an interface is
insufficient. Passthrough remains on its existing strategy path.

## Lifecycle and recovery

Prepare the record for the same bound manifest that the helper uses. A saved
next-start preference cannot revise an existing process's record. Fresh process
start, workspace promotion, and a new process for restore must prepare their
own evidence after a successful write. A restarted Kubernetes helper prepares
its project file through the staged replacement client before configure, so
the record is bound to the replacement instance rather than the retired one.
Rollback discards pending evidence and releases the local or helper-held file
claims.

Same-process load, resume, and reset reuse their original validated record.
They do not imply that Pi rereads a changed file. Live third-party selection
continues to defer for project-file providers, as specified by
[resolution delivery](workspace-mcp-resolution-delivery.md).

Retained agentctl adoption uses its existing adapter and does not create a new
record or session. Do not persist this transient record in execution metadata.
If a new process lacks preparation evidence, admission fails explicitly.
Historical monolithic executions retain their existing mode.

## Attachment evidence

An accepted project entry emits delivery evidence under the current attachment
attempt with a bounded reason such as `project_config_materialized`. It does
not emit an ACP-unsupported failure for that same built-in entry. Deduplicate
HTTP/SSE alternatives by package identity. Preserve ordinary third-party
filter decisions.

Materialized configuration does not prove provider consumption, connectivity,
tool count, or health. Publish none of those observations without their
existing independent evidence. Evidence stays scoped to the current execution,
process, attempt, and package. Use the current sanitization and ordering rules.

## Alternatives and verification

A global capability override misstates Pi's ACP support and marks undelivered
configuration as accepted. An agent-name bypass also accepts skipped or failed
materialization. Both alternatives weaken the existing guard. The prepared
record preserves that guard and the existing lifecycle delivery owner.

These choices fit the established ADR 0020 boundary. This repair needs no new
ADR. The [fix plan](../../../plans/pi-builtin-mcp-delivery/plan.md) defines the
regressions, compatibility matrix, and exact verification commands.
