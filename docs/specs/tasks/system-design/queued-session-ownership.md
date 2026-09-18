---
status: draft
system: tasks
requirements:
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-001
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-002
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-003
  - REQ-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001
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
| REQ-TASKS-QUEUED-SESSION-OWNERSHIP-001 | Inspection intent; Conversation recovery and workflow stop history |
| REQ-TASKS-QUEUED-SESSION-OWNERSHIP-002 | Deferred entry ownership; Task reconciliation |
| REQ-TASKS-QUEUED-SESSION-OWNERSHIP-003 | Queue projection; Desktop and mobile surfaces; Failure and observability |
| REQ-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001 | Replay and reconciliation locking |

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
Keep `launch_queued` and `ownership_unavailable` for pending launch restrictions.
Stop emitting `workflow_parked`; tolerate older payloads during compatibility handling.
Absence means no pending launch restriction. Keep
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

For a recoverable conversation, including a workflow-stopped predecessor, retain the
preference behavior and use automatic ceiling admission. If it is deferred,
retain inspection source in its replay payload and revalidate eligibility on retry.
If a task already has a different accepted launch, inspection cannot replace it
or add a conflicting resume record. An admitted sibling recovery can proceed.
A capacity refusal preserves the accepted record and returns the existing queued
disposition without pretending the selected sibling started.

## Conversation recovery and workflow stop history

Follow [the session-open decision](../../../decisions/2026-09-18-session-open-resumes-conversation.md)
and AC 001.9/003.10. Opening an earlier conversation is sufficient to request
normal automatic recovery. No primary-role or committed-route exception is needed
solely because the conversation was stopped by a workflow switch.

Remove `workflow_parking` and workflow-switch stop-intent checks from
`autoResumeEligibility`. Neither valid, consumed, unconsumed, nor malformed
legacy parking data grants or denies recovery. Task/session authorization,
archive, terminal state, error recovery, and actual pending launch ownership
remain separate checks. Do not infer a new prompt from session opening.

Keep execution-stamped stop-intent parsing and consumed tombstones in the
callback path. They reject delayed events for the old execution. Removing
parking as an activation restriction does not remove event correlation.
Inspect every `workflow_parking` consumer and producer. Remove policy-only
helpers and writes when unused; retain any code with a demonstrated independent
lifecycle purpose. Existing metadata remains inert without a schema migration
or destructive backfill. Do not remove background-work or Office parking.

The shared eligibility predicate still handles deferred launch metadata. Absent,
null, or empty objects represent no pending launch. `stripCeilingRecordKeys`
produces empty objects after settlement. Preserve nonempty-record validation;
do not weaken the replay parser to fix status inspection.

A queued destination remains owned by its accepted launch. Opening it cannot
create another resume. A different selected session can recover with available
capacity, without primary promotion, route changes, or destination prompt delivery.
If capacity is full, preserve the existing deferred record. Do not overwrite it
with the selected sibling's automatic resume. Existing retry and queue mechanisms
remain authoritative; no second task queue or hidden polling loop is introduced.

Status and launch must agree about these rules. Recheck actual queue ownership
at guarded admission after an allowed status response. New parking metadata alone
cannot cancel that permission; a conflicting accepted launch still can.
Keep the lock order and claim fences described below. No runtime call occurs
under task admission.

The browser retains `activation_source=session_open`, preference handling, and
ordinary recovery states. Remove `ParkedSessionNote`, its marker projection, and
its use in `task-chat-panel.tsx`. Remove the unused `parkedSessionNote` locale key
from all catalogs and update affected tests. Do not replace it with another
banner, chip, tooltip, disabled composer, or hidden recovery requirement.
The genuine `TaskLaunchQueueStatus` surface remains task-scoped.

Desktop keeps its session tabs above chat. Phone keeps its existing session
picker and one conversation scroll area. Both show ordinary recovery after open.
Test provider readiness independently of workspace-only readiness. Recovery must
preserve context without resending an interrupted or settled workflow prompt.

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

When a sibling resumes on open or explicitly runs, retain queue status alongside
IN_PROGRESS; settling that sibling restores SCHEDULING. Do not promote it to
primary or run destination entry actions. Keep existing completion-follow-up,
Office, cancellation, and runtime-publication ordering protections.

The sweep also repairs legacy REVIEW+valid-deferral tasks to SCHEDULING when no
session is working. This repair requires authoritative task and entry evidence;
the browser never writes task state based on a badge.

## Replay and reconciliation locking

This correction is implemented by the
[replay deadlock fix package](../../../plans/ceiling-replay-cancellation-deadlock/plan.md).
The implementation and regression results are recorded in that completed
package.

`replayCeilingDeferral` must not retain the task admission lock across a launch,
resume, prompt, runtime wait, or callback publication. The existing
`ceilingDeferredLaunchClaim` owns one accepted record while dispatch is in flight.
Its claim ID and deferral identity protect settlement from successor records.
The claim does not replace workflow-entry validation.

Replay has three boundaries:

1. Under the task admission lock, read and validate the exact record and its
   committed entry. Retain its existing claim and immutable entry binding.
2. Release the task admission lock before the concrete launch seam. Acquire
   existing session lifecycle and cancellation guards in their established order.
   Revalidate entry and recipient at the final admission boundary.
3. Settle or release only the matching claim through existing conditional writes.
   A superseded entry cannot dispatch or clear a successor record.

Final entry validation and its local dispatch claim must serialize with route
mutation. A read before runtime preparation is insufficient. Inspect every replay
kind and its concrete admission seam, including sessionless starts and Send Now.
After preparation, a changed route fails before prompt admission. A route change
after accepted admission follows the existing lifecycle cancellation contract.
No database transaction spans runtime I/O.

`admitCeilingDispatch` validates the entry and renews the existing claim through
one task-admission critical section. The renewal retains the claim ID and uses
the existing conditional write and lease duration. A missing, replaced, or
expired claim rejects dispatch. Claim loss leaves the current record available
to its owner instead of dropping it as a superseded route.

Replay and Send Now carry the claim through preparation in the context. Concrete
launch, resume, prompt, and model-switch boundaries perform final admission.
Prompt admission follows the caller's dispatch-receipt callback. The admission
context never reaches the runtime. Superseded replay cleanup also checks the
claim ID before it removes a deferred record.

Where local critical sections require multiple locks, the order is session
lifecycle, session cancellation guard, task admission, then `taskRuntimeStateMu`.
Acquire only the locks that the operation needs. Never acquire an earlier lock
while holding a later lock. In particular, `writeTaskReviewState` acquires task
admission before the global runtime-state mutex. Task admission holders must not
wait for session guards, runtime callbacks, or event subscribers.

Keep entry validation and task-state writes in a short critical section.
Preserve the existing conditional state write and task-event publication contract.
If a helper publishes synchronously, audit its subscribers before retaining a
lock across that call. Move reentrant publication outside the section while
preserving event order when necessary.

The context marker from `lockCeilingEntryAdmission` represents actual ownership
within that section only. Never pass it to dispatch after release or to another
goroutine. Carry immutable entry identity separately. Do not simulate reentrancy
by attaching the marker to boot-ready callbacks.

Cancellation retains its separate intent marker, shared operation, and bounded
service-owned context. It releases session serialization during provider waits.
Its final task reconciliation must complete after terminal frames settle.
Cancellation policy controls workflow movement, not whether runtime state settles.

Use barrier-controlled service tests for replay, boot-ready, stream activity,
cancellation, and a second queued task. Verify both current task states and
published events. Include mixed sibling states and stale route/claim replacement.
The single ceiling sweep remains sequential and must continue after each settled
attempt. No detached replay workers or new queue mechanism are required.

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
scroll. Do not show a parking-specific note for the selected conversation. The destination's
CREATED start/recovery affordance yields to queued status while accepted work exists.

Use the existing `SessionTaskSwitcherSheet` phone drawer and
`session-mobile-layout.tsx` dedicated composition. Place a compact task queue
region above `MobileSessionsPicker`, outside the chat scroll. The navigator row
shows the same Queued label. Long names wrap in details and truncate in rows.
The user can open Astra while Luna remains named in the task queue region.
Astra follows normal recovery without taking over Luna's workflow ownership.

This is persistent status, so do not add a second drawer or global dashboard.
Keep the existing fixed header/navigation, one chat scroll owner, dynamic viewport,
and safe-area handling. Existing explicit execution controls remain available;
no new bypass button is needed. Status text is keyboard/screen-reader readable,
uses restrained polite announcements for state changes, and never announces every
capacity sample. All new copy uses five-language i18n and the Traditional Chinese
generation command. Previews and viewport assertions live in the work orders.

## Limit scope and configuration navigation

The [opt-in ceiling package](../../../plans/session-ceiling-opt-in/plan.md)
extends the existing live status with explicit limit scope and Settings links
(AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.8-.9). It does not create another queue.

`components/task/launch-queue-status.tsx` renders "Global session limit" and
"Across all workspaces" for `reason: session_capacity`, retaining the selected
destination, sessions-in-use observation, freshness, and retry status. Its
configuration link targets
`/settings/preferences/task-behavior#setting-session-capacity`, the target
registered by the agent-owned Settings work order. Use ordinary internal
navigation, not an execution or task mutation. Members can inspect the setting
but existing permissions prevent writes. Environment-managed settings explain
the lock on arrival; the banner never implies that a link bypasses it.

Ownership-unavailable and replay-error reasons retain their specific explanation;
do not rename them as capacity refusals. An old count can remain labelled stale,
but the global scope is known from the queue kind even without a count. When a
live setting changes, use the existing controller observation and task summary
refresh path. Wake replay on expansion and refresh observations on every applied
change. Do not clear queue ownership just because the effective limit is zero
or its count has room; retain "Retry pending" until confirmed dispatch.

Task details also render the separate WIP queue explanation from the
[WIP design](wip-limit-pull-system.md#limit-updates-and-queue-explanations).
Use task-level composition above conversation content so a WIP task without a
session can expose its reason. Do not put a WIP record into `launch_queue` or
give it the selected-session semantics of global deferral. If independent queue
records legitimately coexist, render their reasons separately with their own
links. Derive neither queue from English message text.

The existing desktop task status region and dedicated phone task layout remain
the surfaces. Links wrap below reason/count text and have at least 44px touch
hit areas. They do not add a scroll owner or require a hover disclosure. Follow
the existing navigation/back behavior; returning to the task remains passive
inspection. Localize labels and link text in all five catalogs.

## Failure and observability

Emit structured, bounded reason codes for suppressed duplicate launch, queued replay,
stale-entry disposition, and protected state reconciliation. Include task,
session, entry, and execution IDs in logs only, with no prompt content.
Do not create lifecycle-only turns merely to announce a suppressed inspection.

Keep historical ceiling notes as history. The live status is independent and
does not rewrite old audit messages. An ordinary deferral does not mean an empty
prompt completed. Verify empty-turn handling at the real event boundary; the
reported warning's exact origin was not proven, so do not remove valid warnings
or claim its cause without a failing regression.

## Related records

- [Conversation recovery decision](../../../decisions/2026-09-18-session-open-resumes-conversation.md)
- [Historical passive inspection decision](../../../decisions/2026-09-16-passive-session-inspection.md)
- [Runtime state publication](runtime-state-publication-order.md)
- [Implementation package](../../../plans/queued-session-ownership/plan.md)
