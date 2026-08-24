# ADR-2026-08-24-first-class-workspace-coordinator: Make Coordinator a durable workspace object

**Status:** proposed
**Date:** 2026-08-24
**Area:** backend, frontend, protocol, security, plugins

## Context

The initial Coordinator design used a hidden workflowless task/session to reuse
the existing agent chat/runtime. That compatibility choice solved transcript,
streaming, and session dispatch, but it incorrectly made a replaceable execution
surface look like the long-lived product object. It does not model explicit
operator authority, auditability, follow-up deadlines, reliable event delivery,
safe-mode, or revocation.

The current Coordinator charter establishes the durable behavior that the
product must preserve: model-independent continuity, bounded helper delegation,
proactive follow-up after silent/rate-limited agents, exact-head PR evidence,
Human-QA receipts, terminal Done integrity, anomaly-loop freeze, and visible
human escalation. Related platform work is separately tracked: master authority
(`6394f111`), relation inspection (`9349b6e5`), own-task inbox (`71b2bc32`),
and narrowly scoped fork credential leases (`16803c08`). None is assumed landed
solely because it is described here.

The current `6394f111` authority schema binds grants to `coordinator_task_id`
and authorizes using the concrete actor task ID. That is not safe for a
replaceable backing task: repair or session replacement would either drop
operator consent or let a task identity become a transferable authorization
handle. This ADR therefore makes a durable principal a prerequisite, rather
than treating it as a naming change to the hidden task.

## Decision

Create a host-owned Coordinator identity scoped to one workspace. The identity
is independent of policy plugins and execution sessions. It stores lifecycle,
operator grants, authority scope, policy version, bounded durable state,
follow-up deadlines, reports, and audit references. It has one authoritative
instance per workspace.

The identity has an opaque server-issued principal ID and is uniquely bound to
`(workspace_id, plugin_installation_id, logical_coordinator_key)`. The host
derives workspace and installation from authenticated context; v1 reserves the
logical key `coordinator`. Plugins cannot choose a task ID as a principal,
transfer a principal, or self-bind a repaired task to one. The host resolves a
principal to its current execution run and optional backing task/session.

Grants bind to the principal, not to a task/session. Each authorization and
audit entry attributes both the principal and the concrete actor
`(run_id, task_id?, session_id?)`. A change in backing task/session requires
host-side actor resolution only, never a grant rewrite. The generic
`AgentConversations` API remains unable to carry principal authority or a
privileged-action request.

Every execution is a separate durable run with trigger, idempotency key, policy
snapshot, retry/backoff state, and an optional backing session. The host manages
event delivery, coalescing, reconciliation, rate-limit recovery, run leases,
session launching/replacement, pause/safe-mode/kill switch, and data retention.
The Coordinator plugin owns orchestration policy, prompts/runbook, report
composition, safe-action selection, and UI composition only.

Privileged board actions remain host-authorized. A plugin cannot self-grant,
widen scope, impersonate another workspace, perform a destructive operation, or
use a credential/security mutation through Coordinator authority. Operator grant
and revoke are explicit, immediately effective for new operations, and audited.
The host records every privileged success, denial, wake, retry, coalescing,
safe-mode transition, and authority change.

V1 selects one durable workspace-scoped Coordinator and one explicit safe
orchestration grant: **Workspace + Assist**. It allows only host-approved,
scope-checked low-risk follow-up actions. V1 has mandatory monitoring for Done
integrity, the Coordinator Todo/follow-up inventory, and pending-human/Human-QA
checks. These signals may produce evidence, a retry, or a visible human ask;
they never confer archive/delete or terminal-cleanup authority. Broader board
control, global scope, and multiple authoritative Coordinators are deferred.

Migration may adopt a legacy plugin/workspace/conversation descriptor as an
unprivileged principal and link its transcript, but it must not migrate a
task-bound grant. The operator re-consents before a principal-bound grant is
issued. Disable stops policy runs while retaining the principal and evidence;
upgrade preserves it only across recognized installation continuity; repair
changes only the execution reference; revoke invalidates all former actors
immediately; uninstall revokes/terminates authority before host-audited
retention or deletion. A repaired, cross-workspace, foreign-installation, or
revoked actor fails closed and cannot restore authority.

Desktop gains a dedicated Coordinator navigation section immediately after
Integrations; mobile projects the same section. Its route presents Overview/
Inbox, Chat, Reports, Activity/Audit, and Settings, with native approval and
anomaly badges. Coordinator is not a plugin item inside Integrations.

`AgentConversations` and `WorkspaceAgentChat` remain generic Stage 0
compatibility primitives. A hidden workflowless backing task/session may serve a
run's chat/runtime needs, but it is never the Coordinator identity, an immortal
session, or implicit proof of authority.

## Consequences

- The host gains additive Coordinator identity, run, grant, audit, event, and
  action-authorization contracts. Existing generic plugin conversations remain
  source- and behavior-compatible.
- Migration adopts an existing plugin/workspace/conversation descriptor into one
  Coordinator record. It must not create a second chat, lose transcript links,
  or silently delete governance evidence on plugin uninstall.
- A minimum implementation can ship identity, read-only Overview/Inbox, pause,
  audit, and dedicated navigation before it grants Workspace + Assist actions.
- Board control depends on the explicit master-authority implementation. Relation
  inspection and own-task inbox remain narrow dependencies rather than a broad
  workspace superuser or message index. Fork credential leases stay a separate,
  mechanical publication capability.
- `6394f111` must replace its task-bound grant/actor lookup with the durable
  principal and host-side execution resolution described here before it can be
  used by Coordinator board control.
- The old plugin-owned scheduler is transitional only. The durable product wake
  source is host events plus periodic reconciliation, with operator-controlled
  subscriptions/cadence and stable idempotency.

## Alternatives considered

- **Permanent hidden task/session.** Rejected as the product identity because
  session failures, replacement, revocation, audit retention, and authority do
  not map safely to one task row.
- **Plugin-owned workspace superuser.** Rejected because a plugin manifest or
  prompt cannot confer revocable, auditable authority and would widen the blast
  radius of a compromised plugin.
- **Taskless utility invocations.** Rejected because they lack durable
  interactive/chat semantics and cannot be the source of run continuity.
- **Plugin-owned cron and polling helpers.** Rejected because wake ownership,
  retries, and lifecycle are unverifiable across restart and model changes.
- **Multiple active coordinators per workspace.** Rejected for v1 because they
  can double-nudge, race, and produce conflicting authority/audit decisions.
  Ephemeral read-only helpers are sufficient for bounded parallel evidence.

## Remaining operator decisions

The v1 scope, multiplicity, Workspace + Assist grant, mandatory monitoring,
and archive/delete exclusion are selected. Before Stage 1 implementation, decide
the approvers and notification surfaces plus audit/report/transcript retention.
The product spec records the remaining choices and acceptance matrix.
