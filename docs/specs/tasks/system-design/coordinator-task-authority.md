---
status: current
system: tasks
created: 2026-08-24
requirements:
  - REQ-TASKS-COORDINATOR-AUTHORITY-001
  - REQ-TASKS-COORDINATOR-AUTHORITY-002
  - REQ-TASKS-COORDINATOR-AUTHORITY-003
---

# Coordinator task authority System Design

## Context and boundaries

Kandev authorization has two tiers. The user/owner tier
(`internal/task/service/service_access.go` `authorizeTaskID`) already lets any
same-workspace task read, message, and move other tasks. The relation tier
(`internal/mcp/handlers/task_target_access.go` `canDirectParentAccess` and
`internal/task/service/handoff_access.go` `canReadDocuments`) restricts four
privileged operations to direct parents or close relations:

| Call site | Operation | Pre-grant rule |
|---|---|---|
| `mcp/handlers/stop_task.go` `canStopTask` | halt a task session | direct parent only |
| `mcp/handlers/handlers.go` interrupt gate | interrupt-delivery message | direct parent only |
| `mcp/handlers/handlers.go` `add_workspace_sources` | attach workspace sources | direct parent only (ADR 2026-08-19) |
| `task/service/handoff_service.go` `ListRelatedForCaller` | relations/documents read | self/ancestor/descendant/sibling/blocker |

This design adds one centralized authority consulted by exactly those four call
sites after the legacy relation check fails. It extends, and does not revoke,
[ADR 2026-08-19](../../../decisions/2026-08-19-parent-authorized-child-workspace-sources.md):
direct parent remains the default fast path and needs no grant. Rationale and
rejected alternatives: [ADR 2026-08-24](../../../decisions/2026-08-24-explicit-coordinator-task-authority.md).

## Data model

Three tables are declared in
[`apps/backend/internal/task/repository/sqlite/coordinator_authority_schema.go`](../../../../apps/backend/internal/task/repository/sqlite/coordinator_authority_schema.go)
and accessed through
[`coordinator_authority.go`](../../../../apps/backend/internal/task/repository/sqlite/coordinator_authority.go):

- `workspace_agent_principals` — durable opaque subject. Identity is
  `UNIQUE(workspace_id, plugin_installation_id, logical_key)`; backing task and
  session are mutable columns. Partial unique indexes ensure one active
  principal owns a backing task or session in a workspace. An empty plugin
  installation or logical key is rejected by the authority, and an empty
  principal id on a legacy task-bound grant never matches the principal-only
  grant query, so mixed deployments fail closed.
- `task_coordinator_grants` — one active grant per `(principal or coordinator
  task, scope kind, scope id)`, enforced by partial unique indexes
  `... WHERE revoked_at IS NULL`. Principal grants are not foreign-keyed to a
  replaceable backing task, so task rotation or deletion cannot erase durable
  operator consent. Capabilities serialize as a normalized comma-separated
  list (`inspect`, `orchestrate`).
- `task_coordinator_audit_events` — append-only privileged-attempt log,
  carrying durable `principal_id` plus concrete actor task/session and using
  claim-then-resolve (`pending` → `ok`/`error`). Pruned at 10k rows with
  dialect-aware `LIMIT -1 OFFSET ?` / `LIMIT ALL OFFSET ?`.

The repository interface is
`CoordinatorAuthorityRepository` in
[`internal/task/repository/interface.go`](../../../../apps/backend/internal/task/repository/interface.go);
schema migrations run for both SQLite and Postgres via
[`internal/task/repository/registry.go`](../../../../apps/backend/internal/task/repository/registry.go).

## Authorization flow

`coordinator.Authority.Authorize`
([`internal/coordinator/authority.go`](../../../../apps/backend/internal/coordinator/authority.go))
executes per request:

1. Direct-parent fast path: allow, return `BasisParent` without reading grant
   state (AC-...-002.1).
2. Runtime flag `coordinatorTaskAuthority` off, or store unavailable: silent
   deny (AC-...-002.2) with no principal DB reads.
3. Resolve the active principal by acting task, then require the request's
   server-authored actor session to equal the principal's current backing
   session. A missing/null plugin context, stale session, store failure, or
   archived actor task fails closed.
4. Cross-workspace actor/target: deny with audit reason `cross_workspace`.
5. Evaluate active grants: matching scope (`workspace`, or `workflow` with
   equal workflow ids) AND required capability present → allow with
   `BasisGrant`; any grant evaluated → audit. Non-matching → deny with a
   scope/capability reason.
6. Audit is written claim-then-resolve. A denied attempt where the principal
   holds no active grant writes nothing (AC-...-001.4).

The `Finish` path resolves the pending audit row to `ok` or `operation_error`
after the privileged call returns. `Decision.DenyReason` is always empty on
return; denial reasons live only in audit rows (`cross_workspace`,
`scope_or_capability`, `archived_actor`) so denials stay opaque
(AC-...-002.3).

Every privileged call re-reads grant state; there is no cache, so revocation
applies to the next operation (AC-...-001.3).

## Repository contract and typed errors

Repository implementations translate dialect-native errors into
`repoerrors` sentinels at the boundary, re-exported from
`internal/task/repository`:

- `ErrWorkspaceAgentPrincipalNotFound` / `ErrWorkspaceAgentPrincipalConflict`
- `ErrCoordinatorGrantNotFound` / `ErrCoordinatorGrantConflict`

Unique-violation attribution lives in
[`sqlite/coordinator_authority_errors.go`](../../../../apps/backend/internal/task/repository/sqlite/coordinator_authority_errors.go)
using the shared `isUniqueViolation` helper (Postgres `pgconn.PgError`
code `23505` + constraint name; SQLite column-list message substring, since
SQLite does not report the index name). Revoke and rebind return the not-found
sentinel when zero rows are affected whether the row is absent or already
revoked, keeping revocation timelines opaque (AC-...-003.4).

Hosts consume this as an adapter: resolve the principal by authenticated
context, ask the authority fail-closed questions with host-loaded
actor/target tasks, then map typed repository outcomes to host-native status
codes. Duplicate-context creates fold to conflict (200-style idempotent
ensure); unknown principals map to not-found (404).

## Runtime flag

`coordinatorTaskAuthority` is registered in
[`internal/runtimeflags/registry.go`](../../../../apps/backend/internal/runtimeflags/registry.go)
and defaults off in the shipped profiles (`profiles.yaml`), with the settings
tab and command-palette entry hidden while off. With the flag off the
authority short-circuits to the legacy relation checks; the empty grant table
is the second layer of default-off.

## Failure and recovery

- Store error during authorize: fail-closed deny, error surfaced to caller;
  the caller maps it to the same opaque denial message.
- Audit claim failure: authorization fails closed before the action. Audit
  resolution failure is surfaced after the action rather than silently
  claiming the row was finalized.
- Revoked or legacy principal rows: treated as absence (silent deny, no
  audit), never as errors.
- Crash between claim and resolve leaves a `pending` audit row, which is
  distinguishable from a completed action and safe to ignore or backfill.

## Testing

Behavior is covered by AC-numbered vectors in
[`sqlite/coordinator_authority_test.go`](../../../../apps/backend/internal/task/repository/sqlite/coordinator_authority_test.go)
(schema and legacy exclusions, grant lifecycle, one-active partial unique,
principal context conflict, rebind/revoke opacity, typed errors, audit
reason-code projection, prune) and by fail-closed, unbound-principal,
store-error, and workflow-scope vectors in
[`internal/coordinator/authority_test.go`](../../../../apps/backend/internal/coordinator/authority_test.go).
Legacy denial byte-parity is asserted at the MCP call sites
(`TestMessageTaskRejectsNonParentInterruptWithoutDisclosure`,
`TestAddWorkspaceSourcesRejectsNonParentCaller`).

## Requirement mapping

| Requirement | Design section |
|---|---|
| REQ-TASKS-COORDINATOR-AUTHORITY-001 | Data model, Authorization flow step 6 |
| REQ-TASKS-COORDINATOR-AUTHORITY-002 | Authorization flow, Repository contract |
| REQ-TASKS-COORDINATOR-AUTHORITY-003 | Data model, Repository contract and typed errors |
