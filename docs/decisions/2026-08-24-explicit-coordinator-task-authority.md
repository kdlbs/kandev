# ADR-2026-08-24-explicit-coordinator-task-authority: Explicit, revocable coordinator task authority

**Status:** accepted
**Date:** 2026-08-24
**Area:** backend, security

## Context

A board-level coordinator task must halt runaway siblings, redirect stalled
top-level tasks, and read relations beyond its own subtree. The relation tier
(direct parent / ancestor / sibling / blocker) only covers messaging and
reads; the four privileged call sites
(`stop_task`, interrupt delivery, `add_workspace_sources`,
`ListRelatedForCaller`) reject unrelated callers. Granting broad
same-workspace authority would let any task mutate any other, and inferring
authority from task titles, agent profiles, prompts, or Office roles is both
unauditable and trivially forgeable by prompt text.

ADR 2026-08-19 established that direct parent is the default for
workspace-source attach. It did not address workspace-wide or
workflow-scoped orchestration by a non-parent.

## Decision

Authority is persisted data, not inference. An operator creates rows in
`workspace_agent_principals` and `task_coordinator_grants` through an
operator-only HTTP surface; no agent-callable MCP tool can mutate grants, so
no self-grant path exists. A principal is identified by the authenticated
context tuple `(workspace, plugin installation, logical key)`, never by its
backing task id, so task rotation and plugin reinstalls preserve identity.
Two capabilities exist — `inspect` and `orchestrate` — and destructive or
credential operations are not representable in that vocabulary, so they can
never be granted.

One centralized server-side authority
(`internal/coordinator.Authority.Authorize`) is consulted after the legacy
relation checks fail. It short-circuits on direct parent first (preserving
ADR 2026-08-19 byte-identically), re-reads grant state on every call (no
cache, so revocation applies to the next operation), requires
`caller.WorkspaceID == target.WorkspaceID == grant.WorkspaceID` at evaluation
time (cross-workspace is impossible by construction), and is gated by the
`coordinatorTaskAuthority` runtime flag, off in every shipped profile.

Denials are byte-identical to the denial a caller without any grant receives;
denial reasons (`cross_workspace`, `scope_or_capability`, `archived_actor`)
are recorded only in `task_coordinator_audit_events`. Audit is written
claim-then-resolve (`pending` → `ok`/`error`) only for privileged attempts by
actors holding an active grant, so an ordinary task's denial produces no row
and audit volume and behavior are unchanged. Legacy task-bound grant rows
(`principal_id = ''`) and null-context principals
(`plugin_installation_id = ''`) are excluded from resolution and fail closed.

This ADR extends, and does not revoke, ADR 2026-08-19: direct parent remains
the default, grants are the declared operator-approved exception.

## Consequences

A coordinator can orchestrate exactly the workspace or workflow an operator
named, and revocation takes effect on the next privileged call. Every
privileged attempt leaves a durable audit row with a reason code, while
out-of-scope and revoked coordinators learn nothing about targets. Hosts
integrate through a typed-error repository contract
(`ErrWorkspaceAgentPrincipalNotFound`, `ErrWorkspaceAgentPrincipalConflict`,
`ErrCoordinatorGrantNotFound`, `ErrCoordinatorGrantConflict`) instead of
string-matching dialect errors.

The cost is a grant read on every privileged call (acceptable: stop,
interrupt, and attach are rare) and an operator-side grant management surface
that did not exist before.

## Alternatives Considered

Inferred authority from task title/profile/prompt/Office role: forgeable by
prompt text and unauditable, so it was rejected. Broad same-workspace
authority: violates least privilege (ADR 2026-08-19's rejected alternative),
so it was rejected. Caching grant reads: stale denials after revocation, so it
was rejected. Letting agents mutate grants through MCP: creates a self-grant
path, so grant mutation is operator-HTTP-only.
