---
status: active
system: tasks
created: 2026-08-24
owners:
  - kandev
---

# Coordinator task authority Requirements

## Overview

A board-level coordinator task needs to orchestrate tasks it is not the parent
of. Relation-tier authorization (direct parent, ancestor, sibling, blocker)
covers messaging and reads but cannot halt a runaway sibling, redirect a
stalled top-level task, or attach workspace sources to an unrelated task.

This capability adds explicit, operator-granted, revocable authority attached
to a durable principal, scoped to a workspace or a single workflow inside it,
checked by one centralized server-side authority, audited on every privileged
action, and defaulting to exactly the pre-grant behavior when no grant exists.
Names, titles, agent profiles, prompt text, and Office roles never confer it.

## Terminology

- **Principal:** The durable, opaque subject an operator approves. A principal
  is identified by the authenticated context tuple `(workspace, plugin
  installation, logical key)`; its backing task and session bindings are
  replaceable implementation details, never the principal's identity.
- **Grant:** A persisted operator decision giving one principal a capability
  over a scope. Grant mutation is operator-only; no agent-callable tool can
  create or revoke grants.
- **Capability:** `inspect` (relation/document reads beyond the relation
  guard) or `orchestrate` (stop, interrupt delivery, workspace-source
  attach). Destructive and credential operations are not grantable.
- **Scope:** `workspace` or `workflow`. Cross-workspace authority is
  impossible by construction.

## Requirements

### REQ-TASKS-COORDINATOR-AUTHORITY-001: Persisted revocable authority grant

**Intent:** Authority is data, not inference. An operator attaches a grant to
a principal; revocation takes effect on the next privileged operation with no
restart.

**User story:** As an operator, I want to grant a coordinator principal
orchestration over a board and revoke it later, so that delegation is
explicit and retractable.

#### Acceptance criteria

- **AC-TASKS-COORDINATOR-AUTHORITY-001.1:** When an operator creates a grant
  naming a principal, a scope kind, a scope id, and a capability list, the
  system shall persist the grant with granter identity and timestamp.
- **AC-TASKS-COORDINATOR-AUTHORITY-001.2:** When a grant is created for a
  principal and scope that already has an active grant, the system shall
  reject the insert with a typed conflict outcome.
- **AC-TASKS-COORDINATOR-AUTHORITY-001.3:** When an operator revokes a grant,
  subsequent authorization checks shall not find the grant, while persisted
  history remains queryable for inspection.
- **AC-TASKS-COORDINATOR-AUTHORITY-001.4:** When a privileged action occurs
  under a grant (allowed or denied while the actor holds any active grant),
  the system shall write an audit event carrying both the durable principal
  id and the concrete actor task/session, then resolve it after the action
  with an ok or error outcome. A denied attempt by an actor holding no active
  grant shall produce no audit event.

### REQ-TASKS-COORDINATOR-AUTHORITY-002: Centralized fail-closed authorization

**Intent:** One authority evaluates direct-parent fast path, then grant scope
and capability. Denials are opaque and byte-identical to the denial a caller
without any grant receives.

#### Acceptance criteria

- **AC-TASKS-COORDINATOR-AUTHORITY-002.1:** When the actor is the direct
  parent of the target, the system shall allow the action without reading
  grant state.
- **AC-TASKS-COORDINATOR-AUTHORITY-002.2:** When the runtime flag is off, no
  grant row exists, the store errors, the principal is absent or revoked, the
  scope does not match, the capability is not granted, the tasks span
  workspaces, or the actor task is archived, the system shall deny the action
  with the same observable outcome as a caller without any grant.
- **AC-TASKS-COORDINATOR-AUTHORITY-002.3:** When authorization is denied for
  any internal reason, the denial response shall not reveal which reason
  applied.
- **AC-TASKS-COORDINATOR-AUTHORITY-002.4:** When legacy task-bound grant rows
  exist (non-transferable rows without a principal), the system shall exclude
  them from principal-based authorization.

### REQ-TASKS-COORDINATOR-AUTHORITY-003: Opaque principal context binding

**Intent:** Hosts resolve a principal from their own authenticated context
(workspace, plugin installation, logical key) rather than from task ids, so
backing task rotation and plugin reinstalls do not change identity.

#### Acceptance criteria

- **AC-TASKS-COORDINATOR-AUTHORITY-003.1:** When a host asks for the
  principal for an authenticated `(workspace, plugin installation, logical
  key)` tuple, the system shall return the principal or a typed not-found
  outcome.
- **AC-TASKS-COORDINATOR-AUTHORITY-003.2:** When two hosts register the same
  context tuple concurrently, exactly one insert shall succeed and the loser
  shall observe a typed conflict outcome.
- **AC-TASKS-COORDINATOR-AUTHORITY-003.3:** When a principal is revoked, the
  system shall stop resolving it as the active principal for its backing
  task, while the row remains readable for status projection.
- **AC-TASKS-COORDINATOR-AUTHORITY-003.4:** When a host rebinds a principal
  to a new backing task and session, the system shall apply the binding to
  the next authorization check and shall reject rebinds of revoked principals
  with the same typed outcome as an unknown principal.
- **AC-TASKS-COORDINATOR-AUTHORITY-003.5:** The system shall authorize only
  the current backing task/session pair, and shall prevent two active
  principals in a workspace from claiming the same backing task or session.

## Out of scope

- Destructive operations (`delete_task`, `archive_task`) and
  credential/security operations are never grantable.
- Grant creation, revocation, and listing use an operator-only HTTP surface;
  no agent-callable (MCP) tool mutates grants.
- Cross-workspace authority and task-ID-based grant fallback.
