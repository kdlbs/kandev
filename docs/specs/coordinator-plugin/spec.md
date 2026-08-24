---
status: proposed
created: 2026-08-17
updated: 2026-08-24
---

# Coordinator

## Product statement

Coordinator is a durable, user-visible workspace product supplied by
`kandev-plugin-coordinator`. It is not an ordinary Kanban task and not an
immortal agent session. The plugin owns its Coordinator policy and durable
product data; Kandev supplies only generic reusable workspace-agent,
authorization, inbox, session, and Automation primitives that the SDK cannot
already provide.

## Approved object hierarchy

```text
Workspace Coordinator — durable identity
├── Policy and operator customization
├── Inbox and pending human decisions
├── Operational state and follow-up ledger
├── Reports and audit history
├── Explicit authority grants
└── Replaceable execution runs
    ├── Primary agent session
    └── Temporary helper sessions
```

There is one authoritative Workspace Coordinator per workspace. The plugin
persists its durable product object, including policy, operator settings, inbox,
decision requests, follow-up ledger, reports, and Coordinator-specific audit
history. A run, task, or session may link to that object, but cannot own or
redefine its durable state. A failed, stale, upgraded, repaired, or replaced
execution resource never creates a second Coordinator or loses its logical
chat, inbox, ledger, reports, or audit history.

## Generic host seams

Kandev core must not grow Coordinator policy, reports, scheduler logic, or a
Coordinator-specific state machine. It provides these minimal generic seams:

- a stable workspace-agent principal/identity with workspace and plugin
  installation provenance, resolved by the server rather than selected by a
  plugin task;
- capability-gated, scope-checked authorization grants and action execution,
  including immediate revocation and host audit attribution;
- a workspace-agent inbox and logical-conversation/session bridge that binds
  successive backing sessions to one durable conversation;
- existing Automations as the only scheduler/event runner, with a typed
  workspace-agent Automation target and idempotent delivery receipt;
- generic lifecycle, storage, route/navigation, and mobile projection hooks.

The Coordinator plugin owns use of these seams: policy/playbook and prompts,
operator customization, monitoring interpretation, Automation templates,
reconciliation logic, follow-up rules, run records, helper policy, reports,
Coordinator audit projections, and UI composition. It never receives raw
database access, broad workspace impersonation, credentials, another task's
queue, or a generic act-as-workspace capability.

### Principal and execution binding

The host issues an opaque generic workspace-agent principal for the tuple
`(workspace_id, plugin_installation_id, logical_agent_key)`. The first
Coordinator consumer uses the reserved logical key `coordinator`; the plugin
presents it as its Coordinator principal. The public core contract should use a
generic name such as `workspace_agent_principal_id`, not make Coordinator a
hidden core superuser. The host derives workspace and installation from
authenticated context and rejects a foreign or duplicate tuple.

Grants bind to the durable principal, never a backing task/session. The host
resolves the principal to a current run/task/session only while executing a
request. Audit records carry both the stable principal and concrete actor tuple
`(run_id, task_id?, session_id?)`. Plugins cannot choose, repair, transfer, or
self-bind actor references. A repaired, foreign-installation, cross-workspace,
or revoked actor fails closed without target-existence disclosure.

Legacy task-bound grants are non-transferable. Migration may link an existing
managed conversation as historical evidence, but creates an unprivileged
Coordinator object until an operator explicitly re-consents to a principal-bound
grant. Disable pauses Coordinator execution; an Automation target becomes
unavailable/paused rather than silently running. Recognized plugin upgrades and
session repair preserve the Coordinator object. Uninstall revokes authority
before the plugin's retention/deletion policy is applied through auditable host
lifecycle operations.

## Authority and safety

The approved v1 authority is explicit **Workspace + Assist** for the workspace
Coordinator principal. Observe is always available; Workspace + Assist permits
only host-approved, scope-checked, low-risk orchestration such as a documented
nudge, proven handoff routing, one safe retry, and Coordinator follow-up. Every
attempt is audited. Broader board control is deferred.

Archive/delete, automatic terminal cleanup, credential changes, authority
changes, merge, history rewrite, and unapproved external/destructive actions
are denied in v1. Pause, safe mode, kill switch, and revocation apply before
the next authorized action without waiting for a session restart.

Mandatory monitoring is limited to evidence-backed **Done integrity**, the
Coordinator **Todo**/follow-up inventory, and **pending-human/Human-QA** work.
These produce evidence, a bounded retry, or a visible human decision; they
never authorize terminal cleanup.

## Automation-driven wakes

Coordinator has no plugin-owned cron, daemon timer, or private scheduler. Its
operator-facing settings guide creation of a Kandev **Automation** targeting
the Workspace Coordinator. The operator selects the target Coordinator,
agent/model, schedule and/or event settings, and a cycle template. Automations
deliver a stable occurrence key; the plugin records/coalesces it in its run
ledger and asks the host to launch/attach a primary session only when needed.

Initial guided templates are:

1. **Wakeup: cycle / board reconciliation** — checks mandatory monitoring and
   due follow-ups.
2. **PR/MR fixup cycle** — responds to selected provider review/CI signals
   within the authorized workspace scope.
3. **Daily standup cycle** — produces the configured concise human report.

The plugin can request an Automation be suggested or validated, but only the
Automation feature owns triggering, schedule/event configuration, retry
delivery, and lifecycle. Provider/model failures, duplicate deliveries, and
missing receipts are represented in the plugin's run ledger and Inbox with a
visible next action; an Automation does not pile messages into a busy session.

## Initial information architecture

Desktop has one top-level **Coordinator** destination immediately after
**Integrations**. It is a workspace-agent destination registered through a
generic host placement primitive, not a Coordinator-specific core section and
not an item nested in Integrations. Phone projects the same destination in the
mobile navigation and opens a direct full-height route.

The initial route has two surfaces:

- **Overview / Inbox** — board health, pending replies and human decisions,
  blockers, due follow-ups, mandatory-monitoring evidence, and next Automation
  action.
- **Chat** — one continuous logical conversation across failed, upgraded,
  stale, repaired, or replacement backing sessions. Session boundaries are
  transcript events, not separate user-facing Coordinator chats.

Settings is reached from this destination and owns policy/customization plus
Automation guidance. Reports and Activity/Audit are durable data owned by the
Coordinator object and may become focused surfaces after the initial release;
they are not required as initial top-level views. Desktop and phone preserve
equivalent Overview/Inbox, Chat, settings, status/badges, direct navigation,
one scroll owner, safe-area-aware controls, zero horizontal overflow, and 44px
touch targets.

## Migration and lifecycle

1. **Stage 0: generic compatibility.** Retain `AgentConversations` and
   `WorkspaceAgentChat` as generic backing-session/chat transport only.
2. **Stage 1: plugin durable object.** The Coordinator plugin creates/adopts
   its Workspace Coordinator state keyed to the generic host principal, links
   legacy chat history, exposes the top-level destination, and keeps execution
   references replaceable.
3. **Stage 2: Automations and continuous chat.** Add generic Automation target
   delivery and logical conversation bridging; the plugin adds the three guided
   templates, run ledger, inbox decisions, and reconciliation policy.
4. **Stage 3: Workspace + Assist.** Depend on generic principal-bound authority
   from `6394f111`; apply every safe action through the host authorizer/audit
   path. Keep terminal cleanup and destructive actions denied.
5. **Stage 4: optional generic expansions.** Add relation inspection
   (`9349b6e5`), own-task inbox (`71b2bc32`), or narrow credential leases
   (`16803c08`) only when the plugin needs their separately scoped capability.

## Dependency and gap matrix

| Item | Classification | Coordinator use |
| --- | --- | --- |
| `AgentConversations` / `WorkspaceAgentChat` | Existing generic host seam | Replaceable session transport and native chat rendering |
| Generic workspace-agent identity/principal and logical conversation bridge | Required minimal host seam | Stable identity and continuous chat without task identity |
| `6394f111` principal-bound authority | Required generic host seam | Workspace + Assist grant/revoke/action audit; its current task-ID grant model must be replaced |
| Kandev Automations target/delivery API | Required generic host seam | All schedule/event wakes; no Coordinator cron in core or plugin |
| `71b2bc32` own-task inbox | Conditional generic host seam | Inbox message integration only, not a board message index |
| `9349b6e5` relation inspection | Conditional generic host seam | Narrow evidence reads only |
| `16803c08` credential lease | Conditional generic host seam | Mechanical publication only; never authority |
| Scheduler, reports, run ledger, reconciliation policy, helper policy, Coordinator UI | Plugin-owned | Must not be added as Coordinator-specific Kandev core functionality |

## Acceptance scenarios

1. Enabling the plugin creates or adopts one durable Workspace Coordinator and
   no visible Kanban task; policy, inbox, ledger, reports, audit, and grants
   remain attached to that object, not any run/session.
2. Replacement, failure, upgrade, or repair of a backing task/session retains
   the logical chat and all durable Coordinator state.
3. The top-level Coordinator destination appears immediately after Integrations
   on desktop and mobile and exposes Overview/Inbox and continuous Chat.
4. An operator can configure an Automation target, agent/model, schedule/event
   settings, and one of the three cycle templates. Duplicate/busy delivery is
   coalesced without a plugin-owned timer.
5. A Workspace + Assist grant is principal-bound and immediately blocks after
   revocation; archive/delete and terminal cleanup remain denied and audited.
6. Done integrity, Coordinator Todo, and pending-human/Human-QA evidence are
   visible in Overview/Inbox and never trigger destructive cleanup.

## Remaining human decisions

Choose approval-notification surfaces and approvers, plus audit/report/chat
retention and export/delete policy. Global scope, multiple authoritative
Coordinators, broader board control, destructive operations, and a
plugin-owned scheduler remain out of scope.
