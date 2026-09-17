---
status: draft
system: tasks
requirements:
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-001
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-002
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-003
---

# Queued session ownership system design

## Purpose and boundaries

This design extends [workflow session lifecycle](workflow-profile-session-lifecycle.md)
at the boundary between an accepted recipient, launch admission, and inspection.
It uses the existing [agent ceiling](../../agents/system-design/session-concurrency-ceiling.md)
and task-owned deferred record. It does not replace either queue or change WIP admission.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-TASKS-QUEUED-SESSION-OWNERSHIP-001 | Inspection intent; Durable workflow parking |
| REQ-TASKS-QUEUED-SESSION-OWNERSHIP-002 | Deferred entry ownership; Task reconciliation |
| REQ-TASKS-QUEUED-SESSION-OWNERSHIP-003 | Queue projection; Desktop and mobile surfaces; Failure and observability |

## Existing components

- `workflow_profile_session_lifecycle.go` parks a source and records an
  execution-stamped stop intent. That record has a durable consumed tombstone.
- `session_launch.go:launchResume` classifies origin from `AutoStart`.
  `buildResumeRequest` currently omits that field for both explicit and open-time recovery.
- `GetTaskSessionStatus` reports runtime resumability. The browser's
  `use-session-resumption.ts` drives recovery after a status read.
- `handleAgentBootReady` calls `setSessionWaitingForInput`, which calls
  `writeTaskReviewState`. The current sibling guard recognizes working sessions,
  but not an eligible CREATED destination whose launch is deferred.
- `ceiling_replay.go` replays by stored launch kind every 20 seconds and on
  release. `ceiling_surface.go` writes historical status messages.
- `task/statussummary` persists a revisioned, bounded summary used by task
  list and detail consumers. Extend it rather than inventing a frontend queue.

## Inspection intent

Add optional `activation_source: "user_action" | "session_open"` to the unified
launch request. Omission preserves existing explicit-call behavior. Reject
unknown values. `session_open` cannot carry a prompt or grant recovery permission.
The backend forces automatic admission for this source even if `auto_start`
was omitted or false. Internal workflow and peer-message paths retain their
existing explicit origins; do not mark all automatic work as passive inspection.

Add `auto_resume_allowed` and `auto_resume_blocked_reason` to session status.
Reasons are closed values: `workflow_parked`, `launch_queued`, or
`ownership_unavailable`; absence means no new ownership restriction. Keep
`is_resumable` for explicit recovery. Populate this through one orchestrator
eligibility helper shared by status, launch, and open-time ensure paths.

The browser consults these fields before `markSessionStarting`. Every passive
resume sends `session_open`; explicit Resume and Send remain user actions.
Inspect `useEnsureTaskSession` and `session.ensure` as well as tab recovery.
An ensure for a task with an accepted queued destination returns that destination
without launching, allocating another session, or switching to the visible tab.

Recheck under the existing session/entry lifecycle guards before reservation,
turn creation, runtime launch, prompt creation, or fresh fallback. Return a
successful no-execution disposition, proposed `activation_disposition:
"suppressed" | "queued"`, with the reason and current session. Teach
`session-launch-service.ts` and recovery operations to accept this as waiting,
not launch success or an error that triggers workspace/fresh fallback.
Transport errors stay errors. Preserve request-generation guards on late responses.

For an ordinary recoverable session without parking or pending work, retain the
preference behavior and use automatic ceiling admission. If it is deferred,
retain inspection source in its replay payload and revalidate eligibility on retry.
If a task already has a different accepted launch, inspection cannot replace it
or add a conflicting resume record. It returns the existing queued disposition.

## Durable workflow parking

Add a typed `workflow_parking` session metadata value with `stamp`,
`parked_at`, and source entry identity where available. Record it even when no
runtime execution exists. This is current policy, not evidence of background work.
Do not reuse `parked_on_background_work` or clear the stop-intent tombstone.

Write parking with the parked session transition under the existing lifecycle
guard and a conditional repository operation. A failed transition must not leave
an unowned parking marker. Clear only the matching stamp when explicit execution
is accepted or a workflow entry commits that session as recipient. No mark is
cleared on a status read, focus, denied admission, or transient launch failure.
Coordinate clear with the launch claim so failed attempts leave the conversation
protected from subsequent passive recovery. A later park supersedes an older clear.

Existing parked tasks may lack this marker. During status/launch eligibility,
recognize an execution-stamped workflow stop intent (including consumed state)
only with non-primary source identity and no newer authorized activation/route.
Inspect durable session execution timestamps and the committed workflow route;
do not equate any WAITING_FOR_INPUT session with parking. If legacy evidence is
ambiguous, deny passive execution with `ownership_unavailable`. Reading remains
possible, and explicit recovery or an authoritative workflow selection resolves
the ambiguity. Never fabricate queue work for an uncertain predecessor.

Keep metadata parsing in task models and conditional writes in the task
repository, with SQLite/PostgreSQL conformance coverage. No new state enum or
database column is required. Tests must include parking without a runtime,
consumed tombstones, later reuse, explicit follow-up, restart, and stale clears.

## Deferred entry ownership

Extend workflow-origin deferred payloads with a nested entry binding containing
workflow ID, destination step ID, committed route operation ID, and destination
session ID. Obtain these from the existing `workflow_session_route` contract,
not asynchronous history. Preserve the first payload and enqueue time across refusals.
Generic start/resume records outside workflow entry keep their own eligibility rules.

Before replay, validate task membership, current entry identity, recipient,
session state, and archive/cancellation status. A superseded, deleted, or terminal
destination receives a final disposition without retargeting. Read failures
retain the record. Old records without the binding are replayable only when the
current committed route and exact session unambiguously agree; enrich them by CAS.
Ambiguous records remain visible as requiring recovery, without dispatch.

Carry the observed record identity through dispatch and clear. A successful
dispatch may clear only that record; it must not re-read and strip a successor's
ceiling keys. Route changes, archive/cancel, and replay claims serialize using
existing task-entry and lifecycle ownership. Never hold a DB transaction while
calling the runtime. Add conditional repository operations where the existing
read-then-write sequence cannot prove atomic ownership.

Recheck the claimed route before prompt admission. Preserve the stored turn and
prompt ownership on retry; a second sweep must not create a second step message.
An already-dispatched exact execution is acknowledged through existing correlated
launch identity. No new scheduler, fan-out queue, or launch lease system is added.

## Task reconciliation

Within `writeTaskReviewState`, use the same task-runtime serialization as running
state reconciliation. Reconcile eligible non-Office, non-archived tasks as follows:

1. Any authorized working session preserves normal IN_PROGRESS behavior.
2. Otherwise, any valid deferred destination preserves or restores SCHEDULING.
3. Only with neither condition may ordinary completion reconcile to REVIEW.

The second predicate is existential: a CREATED queued destination protects the
task even when a sibling is idle, failed, cancelled, or completed. An arbitrary
CREATED row with no accepted launch is not sufficient. Read errors fail closed.
Guard the write against the observed queue/route identity so a concurrent enqueue
cannot lose to an older REVIEW writer. Preserve explicit terminal task actions.

When a user explicitly runs the parked sibling, retain queue status alongside
IN_PROGRESS; settling that sibling restores SCHEDULING. Do not promote it to
primary or run destination entry actions. Keep existing completion-follow-up,
Office, cancellation, and runtime-publication ordering protections.

The sweep also repairs legacy REVIEW+valid-deferral tasks to SCHEDULING when no
session is working. This repair requires authoritative task and entry evidence;
the browser never writes task state based on a badge.

## Queue projection

Add optional `launch_queue` to `TaskStatusSummary` and its frontend type. It is
a complete replacement value under the existing summary revision:

```text
launch_queue: null | {
  session_id?, agent_profile_id?, workflow_step_id?, queued_at,
  reason: "session_capacity" | "ownership_unavailable" | "replay_error",
  retrying: boolean,
  capacity: null | { in_use, limit, observed_at }
}
```

For session-backed workflow launches, session and profile are required. A
sessionless start can use the generic task queue label without fabricating a
session ID; it does not participate in parked-predecessor recipient matching.

Project sanitized identity and status from the durable ceiling record through a
narrow provider at the task-service/orchestrator composition boundary. The task
summary must not import the orchestrator or expose raw replay payloads, prompts,
environment variables, or credentials. Resolve profile names through the existing
authorized profile catalog; fall back to a generic session label if unavailable.

Extend summary rebuild, equality, validation, live projection, boot/list/detail
enrichment, and `task.status_summary.updated`. Include explicit queue removal in
the replacement summary. Missing fields on legacy snapshots are not a newer clear.
Reuse existing revision/invalidation handling in `task-status-summary.ts` and
task hydration; test old list snapshots after a queued update and after a clear.

Admission/refusal/dispatch/drop changes invalidate and rebuild affected summaries.
The existing sweep refreshes observations at most once per pass; use one shared
population sample per pass rather than a count query per row. Capacity is sampled
from the ceiling controller, not copied forever from the original refusal.
Changes to observations do not advance semantic task activity or reorder recent
tasks. On restart rebuild queue ownership first; unavailable capacity is null.
Observations older than two sweep intervals (40 seconds), or a disconnected
client, are labelled stale. UI cannot infer dispatch from a count below the limit.

Queue presence does not increase `queued_prompt_count`: a deferred launch and a
user prompt queue are different contracts. Pending question/error precedence and
existing background-work indicators remain intact. No ordinal position is shown.

## Desktop and mobile surfaces

Share a queue view model and task-scoped status component. Desktop `TaskSwitcher`
rows show Queued text with an existing clock/status icon. Task details place the
status above conversation content, independently of selected session and transcript
scroll. Show a separate parked note for the selected predecessor. The destination's
CREATED start/recovery affordance yields to queued status while accepted work exists.

Use the existing `SessionTaskSwitcherSheet` phone drawer and
`session-mobile-layout.tsx` dedicated composition. Place a compact task queue
region above `MobileSessionsPicker`, outside the chat scroll. The navigator row
shows the same Queued label. Long names wrap in details and truncate in rows.
The user can inspect Astra while Luna remains named in the task queue region.

This is persistent status, so do not add a second drawer or global dashboard.
Keep the existing fixed header/navigation, one chat scroll owner, dynamic viewport,
and safe-area handling. Existing explicit execution controls remain available;
no new bypass button is needed. Status text is keyboard/screen-reader readable,
uses restrained polite announcements for state changes, and never announces every
capacity sample. All new copy uses five-language i18n and the Traditional Chinese
generation command. Previews and viewport assertions live in the work orders.

## Failure and observability

Emit structured, bounded reason codes for suppressed inspection, queued replay,
stale-entry disposition, and protected state reconciliation. Include task,
session, entry, and execution IDs in logs only, with no prompt content.
Do not create lifecycle-only turns merely to announce a suppressed inspection.

Keep historical ceiling notes as history. The live status is independent and
does not rewrite old audit messages. An ordinary deferral does not mean an empty
prompt completed. Verify empty-turn handling at the real event boundary; the
reported warning's exact origin was not proven, so do not remove valid warnings
or claim its cause without a failing regression.

## Related records

- [Passive inspection decision](../../../decisions/2026-09-16-passive-session-inspection.md)
- [Runtime state publication](runtime-state-publication-order.md)
- [Implementation package](../../../plans/queued-session-ownership/plan.md)
