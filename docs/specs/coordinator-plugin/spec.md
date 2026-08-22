---
status: building
created: 2026-08-17
---

# Coordinator plugin

## Overview

The Coordinator is a workspace-scoped supervising agent, shipped as the
`kandev-plugin-coordinator` plugin, that watches monitored workflow steps on a
schedule and reports findings without ever appearing as a Kanban task. It relies
on two new host primitives introduced by this spec: capability-gated
`AgentConversations` (Ensure / Dispatch / Delete) and a host-owned
`WorkspaceAgentChat` UI component. Both primitives are generic — any plugin that
declares the `agent_conversation` capability may use them — and Coordinator is
their first consumer.

## State model

- **Managed conversation.** Identified by `(plugin_id, workspace_id, conversation_key)`.
  Coordinator always uses the stable key `coordinator`. Backed by one workflowless,
  ephemeral task with a single primary session; the task carries server-stamped
  metadata (`kandev.plugin_id`, `kandev.workspace_id`, `kandev.conversation_key`,
  `kandev.ephemeral = true`) that the host — never the plugin — writes.
- **Scheduler due-state.** Per `(workspace_id, trigger)` where `trigger` is
  `monitoring` or `daily`: next-due timestamp, last-success timestamp, and an
  `armed` flag for `monitoring` (false until the first successful `daily` wake).
  Persisted by the plugin so a restart does not re-fire or lose a due occurrence.
- **Occurrence idempotency key.** Every scheduled dispatch carries a stable key
  derived from `(workspace_id, trigger, due_timestamp)`. The host's
  `AgentConversations.Dispatch` claims the key before enqueueing; a retried
  dispatch with the same key returns the prior result instead of re-sending.
  Manual "Run cycle now" / "Run standup now" actions use a separate key derived
  from the manual invocation, never colliding with a scheduled occurrence.
- **Coordinator memory.** Bounded, workspace-scoped structured state: active
  flags, per-task activity snapshots, degradations, last cycle log, last report,
  arming state. Read via `get_coordinator_state` and atomically replaced via
  `publish_report` at the end of a cycle — never inferred from chat transcript.
- **Reports.** Typed, timestamped, paginated artifacts (`cycle`, `daily`,
  `status`) persisted per workspace, visible in the Reports view. The full
  agent response for a wake remains in Chat; the report is the structured
  subset the agent explicitly publishes.
- **Workflow monitoring policy.** Per `(workspace_id, workflow_id, workflow_step_id)`:
  whether the step is checked, and an optional multiline coordinator-specific
  prompt for that step. Stored in Kandev (Settings > Workspace > Workflow
  configuration), read by the plugin through a capability-scoped API, not
  owned by the plugin.
- **Coordinator base prompt.** One installation-scoped, editable string plus
  runbook guidance, stored in Coordinator Settings (plugin config).

## Permissions

- `agent_conversation` capability gates all `AgentConversations` RPCs. A plugin
  without the declared capability is rejected with a typed error before any
  ownership check runs.
- Ownership is enforced per call: a plugin may only Ensure/Dispatch/Delete
  conversations it created (matched by `plugin_id`). Attempting to touch
  another plugin's managed conversation fails the same way as an unknown
  conversation.
- The plugin never receives raw database or private REST access; it reads
  workspaces/workflows/tasks/sessions/messages/agent_profiles through declared
  `api_read` scopes and performs browser-triggered mutations only through its
  own authenticated, workspace-verified actions (`ensure`, `status`, `reports`,
  `run-cycle`, `run-standup`) — never a public webhook.
- `get_coordinator_state` and `publish_report` validate that the invoking
  session belongs to the workspace it claims before reading or writing state.

## Lifecycle

- **Install.** Plugin registers `/coordinator` (`registerRoute`) and one
  Integrations destination (`registerNavItem({ section: "integrations" })`).
  No conversation or task is created at install time.
- **First open / first scheduled tick.** Whichever happens first calls
  `AgentConversations.Ensure`, which idempotently creates or repairs the one
  hidden conversation for that workspace. Concurrent callers (browser open +
  scheduler tick, or a restart mid-creation) converge on the same conversation.
- **Enable/disable.** Disable stops the plugin process and its scheduler;
  the managed conversation, reports, and coordinator state are preserved.
  Re-enable resumes against the same conversation.
- **Config change / upgrade.** Restarts the plugin process and reloads
  configuration and the scheduler without creating a second conversation or a
  duplicate timer.
- **Uninstall.** Stops the process first, then deletes only this plugin's
  managed conversations, reports, and coordinator state through provenance-safe
  host lifecycle code. A cleanup failure is reported, not silently swallowed.
- **Missing/invalid agent profile.** Ensure returns a typed
  configuration-required result; the UI shows a link to settings and never
  creates or dispatches a partial conversation.

## Scheduling

Two triggers per workspace, both timezone-aware (IANA zone, configurable):

- **Monitoring.** Fires every configured interval (default 45 minutes, valid
  range 5-1440) inside the configured window (default 08:00-18:00) on
  configured days (default weekdays). Disabled until the first successful
  `daily` wake has occurred ("arming"). Dispatches `WAKE:CYCLE`.
- **Daily.** Fires once per configured day at the configured time (default
  08:00 America/Montreal). Dispatches `WAKE:STANDUP` followed by the
  configured report template.

Duplicate queued wakes of the same trigger coalesce into one. A busy session
(mid-turn) never accumulates queued heartbeat messages — the dispatch either
joins the in-flight turn's eventual response or is reported as skipped,
depending on the Host's busy-session semantics, and is retried only at the
next eligible cadence or through an explicit manual run.

## Responsive UX

- **Desktop.** Coordinator appears once in the Integrations section of the
  sidebar. Selecting it opens `/coordinator` with native page chrome: a
  Chat/Reports switcher and a settings action. Chat renders through the
  host-owned `WorkspaceAgentChat` component (streaming, queue/cancel,
  clarification, model/command hydration, reconnect-on-reload).
- **Phone.** Coordinator appears once in the mobile Integrations menu and
  navigates directly to the same full-height route (no compressed desktop
  panes), following the layout precedent in
  `apps/web/components/kanban-with-preview.tsx`. Hierarchy is: top bar,
  Chat/Reports switcher, one focused content surface. Chat messages or the
  reports list owns the only internal scroll region; the composer stays
  safe-area-aware and thumb-reachable; settings navigates directly; document
  horizontal overflow stays zero; interactive controls are at least 44px.

## Acceptance scenarios

1. Enabling the plugin on a compatible Kandev version starts its process and
   loads its UI without creating a visible task on any board.
2. Opening Coordinator from Integrations on desktop or mobile opens
   `/coordinator`, not a task route; disabling the plugin removes the nav
   entry and route immediately.
3. Opening Coordinator from two different surfaces concurrently (browser tab
   and a simultaneous scheduler tick) still yields exactly one conversation
   for that workspace.
4. Restarting Kandev mid-`Ensure` self-heals to one conversation on next
   access rather than duplicating or orphaning a partial task/session.
5. Selecting a coordinator agent profile that is later disabled surfaces a
   typed configuration-required state with a settings link; no conversation
   is created or dispatched while unconfigured.
6. A `monitoring` occurrence before the first successful `daily` wake does not
   dispatch (unarmed); after the first successful `daily` wake, `monitoring`
   occurrences dispatch on cadence.
7. Two scheduler ticks for the same due `monitoring` occurrence (e.g. across a
   plugin restart) dispatch exactly once, verified via the occurrence
   idempotency key.
8. A dispatch that arrives while the session is mid-turn does not pile up a
   second queued heartbeat message and is visible as skipped/retried in plugin
   status and Reports, not silently dropped.
9. `publish_report` at the end of a cycle updates coordinator state and, when
   included, a typed report visible in the Reports view; the full assistant
   response for that wake is visible in Chat.
10. Checking a workstep in Settings > Workspace > Workflow configuration and
    saving a workstep prompt causes the next check of that exact workstep to
    receive the composed base prompt + workstep prompt + safety invariants;
    an unchecked step is never evaluated and an empty workstep prompt adds no
    extra instruction.
11. Uninstalling the plugin removes its managed conversations, reports, and
    coordinator state; disabling and re-enabling instead preserves all three.
12. A second, unrelated plugin holding a different capability set cannot
    Ensure, Dispatch, or Delete the coordinator's managed conversation.

## Out of scope

See plan section 11 (Out of Scope) for the authoritative list: multiple
coordinator instances per workspace, autonomous heartbeat tuning, transcript-
parsed reports, marketplace publication, and changes to Kanban workflow
semantics are explicitly excluded from this build.
