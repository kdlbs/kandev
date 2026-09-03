---
status: current
system: platform
requirements:
  - REQ-PLATFORM-BACKGROUND-WORK-LIVENESS-001
---

# Background Work Liveness System Design

## Purpose and boundaries

The Platform system owns the public session-activity projection because it
combines the durable session lifecycle with process-local activity accounting.
This design covers backend serialization, complete session-list snapshots,
partial WebSocket events, client reconciliation, and restart recovery. It does
not change background-work detection, prompt admission, agent resumption, the
Claude experiment, or status-component layout and copy.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.1` to `.3` | [Backend activity authority](#backend-activity-authority), [Client update classes](#client-update-classes) |
| `AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.7` and `.8` | [Backend activity authority](#backend-activity-authority), [Concurrent refresh and event ordering](#concurrent-refresh-and-event-ordering) |
| `AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.9` | [Restart recovery](#restart-recovery), [Concurrent refresh and event ordering](#concurrent-refresh-and-event-ordering) |

## Backend activity authority

The orchestrator's process-local tracker remains the authority for the current
public `foreground_activity` value. Session DTO enrichment combines that value
with the durable coarse state and the Claude experiment policy. A `RUNNING`
session exposes `generating` by default. An eligible Claude session can expose
`background`. When no public activity applies, the session DTO omits
`foreground_activity` through its existing `omitempty` contract.

The omission is meaningful in a complete session record. It means that the
backend has no public activity for that session at the instant it built the
snapshot. It is not a compatibility signal that asks the client to retain a
value from an earlier backend process.

## Client update classes

The client handles two update classes with different omission semantics:

- `GET /api/v1/tasks/{task_id}/sessions` and task-detail boot data contain
  complete session records. An omitted `foreground_activity` replaces the
  previous projection with no activity.
- `task_session.state_changed` and other WebSocket updates can contain partial
  session records. An omitted field means no change. An explicit activity
  value or `null` updates or clears the projection.

The generic partial-session merge keeps its omission-preserving behavior for
WebSocket events and narrow local updates. The authoritative session-list
action normalizes an omitted activity field to the client clear value before
it merges the complete record.

The activity projection reconciled as one unit consists of
`foreground_activity`, `active_subagent_count`, and `supports_steering` because
one activity event publishes all three fields from the same runtime snapshot.

## Concurrent refresh and event ordering

An HTTP response can be older than an activity event that arrives while its
request is in flight. The session slice therefore maintains a process-local,
per-session client activity epoch. Applying an explicit activity projection
from a WebSocket state or activity event increments that epoch, including when
the value repeats or clears.

Every client-side asynchronous session-list loader captures each existing
session's epoch through the shared activity-epoch helper before starting its
request. This includes normal task hydration, task-selection/removal loading,
and Office task detail loading. When the response arrives:

1. If the epoch did not advance, the complete response replaces the activity
   projection. An omitted `foreground_activity` clears the stored value.
2. If the epoch advanced, the event projection is newer than the request. The
   client keeps the current activity projection while it merges the response's
   durable session fields.
3. A session first observed in the response adopts the response projection
   directly.

The complete-snapshot action requires a request-start epoch map. There is no
unguarded asynchronous call shape; synchronous tests and utilities must also
state their boundary explicitly. Removing a session also removes its client
activity epoch.

This ordering rule protects the opt-in Claude background tier while allowing a
post-restart snapshot to clear activity retained from the previous process.

## Restart recovery

Background-work accounting is not persisted. After backend restart, a settled
session record normally omits `foreground_activity`, even if the browser still
holds `background` from the old process. The reconnect and foreground-refresh
paths already fetch the complete task-session list. The authoritative merge
uses the omission to clear the old projection. The coarse
`WAITING_FOR_INPUT` state then renders as ready for input rather than as active
background work.

The resume command and the `Resumed agent` transcript row report process
reconnection only. They do not establish a foreground model turn and do not
set client activity by themselves.

## Failure and recovery

A failed refresh retains the last usable session state and remains retryable.
It does not advance or clear the activity projection. A later reconnect,
foreground event, or successful foreground refresh can reconcile the session.

If a WebSocket event introduces a session that was absent when the request
started, the existing added-during-load guard keeps that session and schedules
a follow-up refresh. Activity epochs use the same request boundary and do not
replace that identity guard.

Forced task hydration also owns a synchronous in-flight guard. Two reloads
started before React observes the loading-state update are serialized, so an
older response cannot race a newer response into the store.

## Verification strategy

Session-slice tests prove that a complete settled snapshot clears an omitted
activity field while a partial event preserves it. Hook, task-selection, Office
detail, and slice tests hold a session-list request open, apply a newer activity
event, and prove that the older response cannot erase the event projection.
Existing explicit-value, explicit-null, cancellation-revision,
route-generation, and newly-added-session tests remain green.

A desktop Playwright case extends the backend-restart session-resume flow. It
starts with a retained background projection, restarts the backend, opens or
refreshes the resumed settled session, and verifies that the status and
composer no longer report background work. No separate mobile E2E case is
required because this change only reconciles shared store data and does not
change layout, touch interaction, navigation, or responsive behavior.

## Related decisions

- [Restore Coarse Running Prompt Admission](../../../decisions/2026-07-28-coarse-running-busy-signal.md)
- [Fine-grained foreground-idle busy signal](../../../decisions/0049-fine-grained-foreground-idle-busy-signal.md)
