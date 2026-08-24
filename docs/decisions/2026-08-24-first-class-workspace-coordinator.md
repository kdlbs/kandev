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

## Decision

Create a host-owned Coordinator identity scoped to one workspace. The identity
is independent of policy plugins and execution sessions. It stores lifecycle,
operator grants, authority scope, policy version, bounded durable state,
follow-up deadlines, reports, and audit references. It has one authoritative
instance per workspace.

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
  audit, and dedicated navigation before it grants board-control mutation.
- Board control depends on the explicit master-authority implementation. Relation
  inspection and own-task inbox remain narrow dependencies rather than a broad
  workspace superuser or message index. Fork credential leases stay a separate,
  mechanical publication capability.
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

## Open operator decisions

Before Stage 1 implementation, decide workspace-only versus a later global
aggregate, the allowed autonomy tiers and approvers, audit/report/transcript
retention, notification surfaces, and the initial event-source set. The product
spec records recommendations and the acceptance matrix.
