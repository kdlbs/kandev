---
status: proposed
created: 2026-08-17
updated: 2026-08-24
---

# Coordinator

## Product statement

Coordinator is a durable, workspace-scoped board-lead product. It helps an
operator supervise work, route safe follow-up, preserve evidence, and surface
the decisions that genuinely require a human. It is not a Kanban task, a
standing chat session, or a plugin-created superuser.

The Coordinator plugin supplies the orchestration playbook, prompts, reports,
and page composition. Kandev owns the durable Coordinator identity, authority,
audit, wake delivery, workspace boundary, and execution-run lifecycle. A hidden
workflowless task/session may remain an internal compatibility adapter during
the first implementation stage, but it is never the product identity or the
source of authority.

## Scope and identity

There is one authoritative Coordinator identity per workspace. Its durable
record is host-owned and contains:

- stable coordinator ID and workspace ID;
- lifecycle state: `disabled`, `active`, `paused`, `safe_mode`, `revoking`, or
  `deleted`;
- selected plugin/policy version, configured profile policy, monitoring scope,
  retention policy, and current safety mode;
- explicit operator grants and revocations, including the scope and expiry of
  any board-control authority;
- follow-up deadlines, active flags, bounded memory, report references, and
  the last reconciliation cursor;
- append-only audit references for authority changes, wakes, policy decisions,
  privileged actions, and destructive-operation denials.

The identity is separate from a replaceable **execution run**. A run has its
own ID, trigger, idempotency key, policy snapshot, input-event references,
state (`queued`, `running`, `waiting`, `succeeded`, `failed`, `cancelled`, or
`coalesced`), attempt/backoff information, and optional backing session. A
failed, rate-limited, or superseded session never destroys Coordinator history
or makes a replacement run a new Coordinator.

Only one run may hold a coordinator's mutation lease at a time. Read-only
evidence helpers are non-authoritative, short-lived, bounded to the specific
request, and receive no implicit board-control grant.

### Principal contract and execution binding

The Coordinator record is also the durable authorization **principal**. Its
opaque, server-issued `coordinator_principal_id` is bound to exactly the tuple
`(workspace_id, plugin_installation_id, logical_coordinator_key)`. For v1 the
only accepted logical key is the host-reserved `coordinator`; it is not a
plugin-controlled task ID or a display name. The host derives the workspace and
installation from the authenticated caller/installation registry, rejects a
foreign or duplicate tuple, and returns the same principal on retries.

The host alone resolves a principal to a current execution run and, when a run
needs chat transport, to a replaceable backing task/session. Plugins receive
typed descriptors and may request a run or safe action for their own resolved
principal; they cannot set, repair, or transfer the principal's actor binding.
The current task/session is an internal execution reference, never grant
identity. Replacing a crashed, deleted, or partial backing task therefore
preserves the principal while requiring the new actor to be resolved by the
host before it can act.

Every authorization and audit record carries both the stable principal and the
concrete actor tuple `(run_id, task_id?, session_id?)`. Host API shape is
additive and capability-scoped: operator-only create/grant/revoke/delete
operations; plugin-readable descriptor/status/audit-page operations; and
host-authorized `StartCoordinatorRun` and `ExecuteCoordinatorAction` requests
that resolve the actor server-side. `AgentConversations` remains a separate
generic API and cannot accept a principal, a grant, or a privileged action.

No legacy task-bound grant is silently upgraded. Migration can adopt a legacy
plugin/workspace/conversation descriptor as an unprivileged principal and link
the old transcript as evidence, but the operator must explicitly re-consent to
issue a principal-bound grant. A revoked grant invalidates every future action
immediately, including actions attempted by a previously bound task/session;
repair, retry, or a plugin restart cannot restore it. Disable stops policy runs
without changing retained identity/audit data; upgrade preserves the principal
only when the registered installation is the same continuity-recognized
installation; repair replaces only execution references; uninstall first
revokes/terminates authority and then follows the selected retention/deletion
policy through a host-audited operation.

## Authority and safety model

The host authorizes privileged board actions. The plugin proposes an action;
the host validates the current grant, workspace/workflow scope, action class,
target ownership, policy tier, and idempotency before it executes anything.

The baseline has three operator-visible autonomy tiers:

1. **Observe.** Read state, maintain an inbox, publish reports, and recommend.
   It cannot mutate tasks or interrupt runs.
2. **Bounded lead.** Perform configured low-risk orchestration actions within a
   selected workflow, such as a documented nudge, forward routing of a proven
   handoff, or one safe retry. Actions are audited and reversible where the
   underlying task contract permits.
3. **Board control.** Exercise individually granted parent-equivalent actions
   in the declared workspace/workflow, including interrupt, redirect, and
   recovery of a stalled task. This tier remains unable to delete/archive,
   change credentials, grant authority, merge, rewrite history, or perform an
   unapproved external/destructive action.

`safe_mode`, pause, revocation, and kill switch take effect before the next
privileged operation, without requiring the plugin process or a session to
restart. Pausing preserves identity, history, inbox, reports, and audit data;
it stops new autonomous mutation runs. A human can still inspect and manually
start an appropriately authorized read-only or diagnostic run.

No task title, prompt, profile, plugin manifest, or Office role can confer
Coordinator authority. The operator grants and revokes it through a host-owned
surface. All grant attempts, denials, and privileged action results are audited
with actor coordinator/run/session, target, timestamp, declared reason, policy
version, and result. Cross-workspace and out-of-scope targets fail without
leaking their existence.

## Event and reconciliation model

Coordinator is event-driven with periodic reconciliation, not cron-owned by a
plugin session. The host publishes normalized, workspace-scoped events and
coalesces them into durable wake records. Candidate inputs are task and workflow
transitions, session state/error changes, clarification/flag changes, queued
messages for the Coordinator's own task-scoped inbox, provider CI/review
changes, grant/revocation changes, and due follow-up deadlines.

The host assigns every occurrence an idempotency key and records whether it was
accepted, coalesced, deferred, or rejected. Periodic reconciliation catches lost
events, validates terminal Done receipts, expires follow-up deadlines, and
restores a run after a host/plugin restart. The operator controls cadence and
event subscriptions; the Coordinator cannot create an untracked scheduler.

Provider and model failures are first-class run outcomes. The host records a
typed rate-limit or provider-retry deadline. Before that deadline, duplicate
wakes coalesce. At the first eligible reconciliation, one retry occurs; after a
second missing receipt the configured fallback applies: safe-mode, a human
approval request, or a bounded alternate authorized run. Successful transport
does not close a follow-up; only the requested evidence or explicit reply does.

## Information architecture

Desktop navigation contains a dedicated **Coordinator** section immediately
after **Integrations**. It is a host-owned section, not an item inside the
plugin's Integrations contribution. The section shows the active Coordinator
destination and compact badges for pending approvals, unread inbox items, and
actionable anomalies. It is hidden when no Coordinator identity is enabled for
the active workspace.

The Coordinator route has five focused surfaces:

- **Overview / Inbox** shows current safety state, pending approvals, due
  follow-ups, queued messages for the Coordinator's own task, active flags,
  degraded capabilities, and recommended next actions.
- **Chat** shows human interaction and the selected run's transcript. It is not
  the durable source for reports, grants, or audit decisions.
- **Reports** shows typed cycle, daily, status, and terminal-integrity artifacts
  published as structured data.
- **Activity / Audit** shows wake/run history, privileged actions, denials,
  policy/grant changes, retries, coalescing, and links to evidence.
- **Settings** lets an operator configure policy, monitored worksteps, prompts,
  event subscriptions, schedule/reconciliation cadence, retention, autonomy
  tier, grants, pause/safe mode, and kill switch.

Phone navigation projects the same Coordinator section in the mobile menu and
opens a direct full-height route. The top bar carries the section title and
approval/anomaly count. Overview/Inbox, Chat, Reports, Activity, and Settings
are individual focused views rather than compressed desktop panes. Each view has
one scroll owner, safe-area-aware controls, zero document horizontal overflow,
and 44px minimum touch targets. Notifications deep-link to the relevant inbox,
approval, report, or audit record in the active workspace.

## Host and plugin ownership

| Host owns | Plugin owns |
| --- | --- |
| Coordinator identity, lifecycle state, workspace isolation, operator grant/revoke, policy-tier enforcement, audit, destructive boundaries, event subscriptions, durable wake records, idempotency, follow-up timers, rate-limit recovery, run/session launch, and storage retention | Board-leading policy, versioned prompts/runbook, workstep-specific instruction composition, triage/recommendation logic, safe-action selection, structured reports, Coordinator page composition, and presentation of host-provided state |
| Typed action execution and denial, event redaction, task-scoped inbox enforcement, session replacement/recovery, and lifecycle cleanup | Requests for runs and scoped actions, bounded memory/report publication, and plugin-specific non-privileged configuration |

The host/plugin API must expose typed coordinator descriptors, run descriptors,
grant descriptors, audit pages, inbox pages, wake receipts, and policy/action
requests. It must not expose raw database access, broad message queues,
credentials, private documents, unscoped provider events, or a generic
"act-as-workspace" capability.

## Compatibility and migration

1. **Stage 0, current foundation.** `AgentConversations` and
   `WorkspaceAgentChat` remain generic plugin primitives. The existing hidden
   workflowless task/session is retained only as a compatibility-backed chat and
   execution adapter. It has no implicit Coordinator authority.
2. **Stage 1, host identity and grants.** Add a host-owned Coordinator record,
   explicit operator grant/revoke UI/API, audit store, dedicated navigation
   section, and migration that adopts an existing Coordinator plugin's
   workspace/key descriptor without creating a second chat or inheriting a
   task-bound grant. Bind grants to the server-issued Coordinator principal;
   require explicit operator re-consent for any legacy authority.
3. **Stage 2, runs and wakes.** Add durable run records, event subscriptions,
   reconciliation, follow-up deadlines, typed provider/model recovery, and
   pause/safe-mode/kill switch. The adapter launches or repairs a backing
   session for a run only when required.
4. **Stage 3, privileged board control.** Depend on the master-authority and
   same-workspace relation-read contracts. Route every privileged action through
   the centralized host authorizer and audit path. Keep destructive and
   credential actions separately denied.
5. **Stage 4, plugin adoption.** The Coordinator plugin consumes the new
   descriptors and event/wake contract, migrates its scheduler state into host
   records, and treats earlier transcript/chat history as linked evidence.

Disable, update, restart, and temporary plugin unavailability preserve the
Coordinator identity, policy, run history, reports, inbox, and audit record.
Uninstalling the policy plugin disables the identity and stops new policy runs;
retention/deletion follows the operator-selected policy rather than silently
deleting governance evidence. A later deletion is an explicit host-owned,
audited destructive operation.

## Dependencies

- **6394f111**, master Coordinator authority, is required before Stage 3. Its
  current task-ID grant model is insufficient: it must instead bind explicit,
  revocable, audited, scope-limited authority to the durable Coordinator
  principal and server-resolve the concrete execution task/session.
- **9349b6e5 / PR #2841**, relation inspection, is a narrow read dependency.
  It is under review and must not be assumed available by any design or test.
- **71b2bc32 / PR #2974**, task-scoped inbox, is required for the inbox's
  queued-message slice. It remains own-task-only and does not become a board
  message index.
- **16803c08 / PR #2940**, fork credential leases, is an optional mechanical
  publication dependency. It must never grant the Coordinator broad credential
  authority.
- The current `AgentConversations`/`WorkspaceAgentChat` contract is a Stage 0
  compatibility dependency, not the identity or authorization design.

## Threat model and invariants

- A compromised or buggy plugin cannot self-grant, widen scope, impersonate a
  different workspace, bypass pause/revocation, delete/archive, expose
  credentials, or turn a helper into an authoritative Coordinator.
- Replacing, repairing, deleting, or replaying a backing task/session cannot
  bind it to a different principal, retain an old grant, or hijack a principal
  in another workspace. The host rejects actor/principal/installation mismatch
  without existence disclosure.
- A replayed event/run cannot duplicate a privileged action after its
  idempotency key is claimed. A stale or rate-limited session cannot accumulate
  unbounded wake messages.
- Cross-workspace reads/writes, private documents, and another task's queue are
  denied without existence disclosure.
- Every privileged attempt, including denial and safe-mode block, is auditable.
  Audit data is retained independently of replaceable sessions and plugin
  upgrades.
- Terminal Done integrity is evidence-backed. A merged PR, a chat claim, or a
  workflow column alone never authorizes destructive cleanup.
- Anomaly loops freeze instead of retrying indefinitely. Human-only decisions
  are visible as approvals, not buried in a transcript or cycle report.

## Acceptance scenarios

1. Enabling Coordinator in a workspace creates or adopts exactly one durable
   Coordinator identity and no visible Kanban task.
2. A replacement chat/run session after restart retains identity, policy,
   inbox, reports, audit history, and pending follow-ups.
3. An ordinary task, a revoked Coordinator, and a Coordinator from another
   workspace cannot perform a board-control action; each result is audited
   without leaking target details.
4. An operator can grant a bounded tier, observe its effective scope, pause or
   revoke it, and see the change block the next privileged action immediately.
5. Task, provider, inbox, and due-follow-up events coalesce into one durable
   wake; periodic reconciliation recovers a dropped event and records why.
6. A rate-limited run follows its typed retry deadline, does not pile up wake
   messages, and eventually uses the configured fallback or visible approval.
7. Desktop and mobile show the dedicated Coordinator section after Integrations,
   the five focused surfaces, notification/approval badges, and equivalent
   direct navigation without horizontal overflow.
8. Activity/Audit can explain every privileged action, denial, retry,
   coalescing, safe-mode transition, and terminal-integrity recovery with stable
   identifiers and evidence links.

## Human decisions required before Stage 1

- **Scope:** workspace-only is recommended for v1. A global coordinator needs a
  separate aggregate identity, cross-workspace consent, and a different threat
  model.
- **Autonomy:** adopt the three tiers above, or require per-action approval for
  all mutation. Recommendation: Observe by default, Bounded lead opt-in, Board
  control only through an explicit scoped grant.
- **Multiplicity:** one authoritative Coordinator per workspace is recommended.
  Decide whether read-only named assistants may exist as separate product
  identities or only as ephemeral helpers.
- **Approval UX:** decide whether approvals live only in Overview/Inbox or also
  generate native task/desktop/mobile notifications, and who may approve them.
- **Retention:** choose duration and export/delete policy for audit, reports,
  inbox metadata, and linked session transcripts.
- **Event sources:** select the v1 sources from task/workflow, session/error,
  own-task inbox, provider PR/CI, and deadline events. Each added provider
  source needs explicit data minimization and retry semantics.

## Out of scope until approved

Global cross-workspace coordination, multiple authoritative Coordinators in one
workspace, autonomous credential/security changes, destructive task operations,
unbounded helper swarms, transcript scraping as governance storage, and a
plugin-owned hidden scheduler are out of scope.
