# ADR-2026-08-24-first-class-workspace-coordinator: Make Coordinator a durable plugin-owned workspace object

**Status:** proposed
**Date:** 2026-08-24
**Area:** plugins, backend, frontend, protocol, security

## Context

The initial Coordinator design used a hidden workflowless task/session to reuse
agent chat/runtime. That compatibility choice is useful transport, but it is
not safe as the long-lived product identity: a task/session may fail, be
repaired, or be replaced. It must not own Coordinator policy, authority, inbox,
follow-up state, reports, audit history, or logical chat continuity.

The product needs one user-visible Coordinator per workspace, bounded helper
delegation, durable human decisions/follow-ups, exact-head and Human-QA
evidence, anomaly-loop freeze, and explicit safe authority. These are
Coordinator policy and must remain in `kandev-plugin-coordinator`, not become a
Coordinator subsystem in Kandev core. Kandev core should add only reusable
workspace-agent primitives missing from the SDK.

`6394f111` currently binds grants to a concrete task. That is unsafe for a
replaceable execution resource and must become a generic principal-bound grant
before it can authorize Coordinator actions. `9349b6e5`, `71b2bc32`, and
`16803c08` remain narrow conditional dependencies, not implied workspace
superuser capabilities.

## Decision

The plugin owns a durable **Workspace Coordinator** object with this hierarchy:

```text
Workspace Coordinator
├── policy and operator customization
├── inbox and pending human decisions
├── operational state and follow-up ledger
├── reports and Coordinator audit history
├── explicit authority grant references
└── replaceable execution runs
    ├── primary agent session
    └── temporary helper sessions
```

One object exists per workspace. Runs/tasks/sessions are replaceable resources;
they link to the object but do not store authoritative Coordinator state. The
host supplies a generic opaque workspace-agent principal for the plugin
installation/workspace/logical-key tuple and resolves it to a concrete actor at
execution time. Core API naming must remain generic (for example
`workspace_agent_principal_id`), even though the first consumer calls it its
Coordinator principal. Grants bind to this principal, never a task/session;
audit records both principal and concrete run/task/session actor.

The approved initial authority is explicit **Workspace + Assist**: host-checked
low-risk orchestration in the selected workspace. Observe is default. Archive,
delete, terminal cleanup, credentials, authority changes, merge, history
rewrite, and other destructive/external actions remain denied. Mandatory
monitoring covers Done integrity, Coordinator Todo/follow-up, and
pending-human/Human-QA work. Such checks produce evidence, a bounded retry, or
a visible human ask, never destructive cleanup.

The Coordinator has no private cron or scheduler. It guides the operator to
create an existing Kandev **Automation** targeting the Workspace Coordinator,
choosing agent/model, schedule/event settings, and one of three templates:
board reconciliation, PR/MR fixup, or daily standup. Automations own triggers,
retry delivery, and lifecycle; the plugin owns the template, occurrence/run
ledger, coalescing interpretation, follow-up policy, and report/UI result.

The initial UI is one top-level Coordinator destination after Integrations,
projected on desktop and mobile by a generic workspace-agent navigation seam.
It starts with Overview/Inbox and one continuous logical Chat. Settings supplies
policy customization and Automation guidance. Reports and Activity/Audit remain
durable plugin data and can gain focused views later. A logical-conversation
bridge makes backing-session replacement transparent, while representing
session boundaries in the transcript.

## Consequences

- Kandev core provides only generic workspace-agent identity/principal,
  authorization/action execution, inbox, logical-session bridge, Automations
  target/delivery, storage/lifecycle, and navigation primitives. Existing
  `AgentConversations` and `WorkspaceAgentChat` remain generic compatibility
  transport, not Coordinator authority.
- The plugin owns Coordinator persistence, policy, reports, audit projection,
  run ledger, helper rules, reconciliation, Automation templates, and UI.
- Legacy managed conversations may be linked as history, but legacy task-bound
  grants are non-transferable: the operator must re-consent. Disable pauses the
  plugin target, upgrade/repair preserve the durable object, revoke fails closed
  for every prior actor, and uninstall revokes before retention/deletion.
- A first release can ship a plugin-owned object and Overview/Inbox/continuous
  Chat once generic identity, logical conversation, navigation, and Automation
  target seams exist. Workspace + Assist waits for `6394f111` to generalize its
  principal-bound authorizer.

## Alternatives considered

- **Permanent hidden task/session.** Rejected as product identity because it
  makes repair/replacement threaten policy, consent, audit, and continuity.
- **Coordinator-specific core state machine/scheduler.** Rejected because it
  hard-codes one plugin's policy and duplicates Automations.
- **Plugin cron/daemon timer.** Rejected because scheduling, trigger retries,
  and lifecycle belong to Kandev Automations.
- **Plugin workspace superuser.** Rejected because prompts/manifests cannot
  confer revocable, audited authority.
- **Multiple authoritative Coordinators.** Rejected for v1 because they race
  on follow-up and authority; helpers remain temporary and non-authoritative.

## Remaining decisions

The human selected workspace scope, one Coordinator, Workspace + Assist,
mandatory monitoring, archive/delete exclusion, initial navigation, and the
two initial surfaces. Remaining decisions are approval-notification surfaces,
approvers, and audit/report/chat retention and export/delete policy.
