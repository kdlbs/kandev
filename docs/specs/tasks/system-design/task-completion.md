---
status: draft
system: tasks
requirements:
  - REQ-TASKS-COMPLETION-001
  - REQ-TASKS-COMPLETION-002
---

# Task Completion System Design

## Purpose and boundaries

The task system owns workflow completion and conversation admission. Runtime
shutdown releases execution resources; it does not establish permanent loss of
conversation access. Agent runtime restoration and workspace recovery retain
their current owners and authorization checks.

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-COMPLETION-001` | Step contract, Migration, Entry and notification |
| `REQ-TASKS-COMPLETION-002` | Explicit resume, Follow-up ownership, Cleanup and races, User interface |

## Step contract

Use `CompleteTaskOnEnter bool` and `complete_task_on_enter` in JSON, YAML, and
SQL. The naming follows `cancel_triggers_turn_complete`. Responses and current
exports include false explicitly. PATCH uses presence-aware decoding: omission
preserves the value, false disables it, and explicit null is rejected.
Create defaults an omitted value to false and rejects null or non-booleans.

Completion requires both final-step eligibility and an enabled setting.
Persist the setting on its step across reorder operations, but ignore it while
that step is non-final. The new final step uses its own saved value; reordering
does not transfer or automatically enable completion. The editor derives
visibility from the current draft order, and the backend evaluates the committed
order. Saving or discarding the order and settings uses existing coordination.

Carry the field through both task and workflow step models, `StepDefinition`,
the embedded workflow loader, task DTO conversion, workflow controller request
types, `stepevents`, MCP schemas and handlers, and all repository projections.
`workflow_steps.complete_task_on_enter` uses the repository's integer boolean
convention with a non-null zero default. Both repositories initialize this
shared table; neither constructor may erase or bypass migration decisions.

Frontend types, workflow API, boot hydration, WebSocket merges, step drafts,
save/rollback, workflow creation, and duplication carry the same boolean.
Partial WebSocket updates that omit it retain the saved value.

Portable export advances to version 2. Each version-2 step requires a boolean
completion field. Import and sync accept versions 1 and 2. At the version-1
boundary only, omitted values are materialized using the old name-and-final
predicate; explicit false always wins. Reject null in either version.
Normalize the whole workflow before converting individual steps, because legacy
final-position detection needs all steps. Export always emits version 2.
Thus current authored definitions never depend on names at runtime.

Sync applies normalized values and includes the field in equality checks.
Version-1 sync files remain authoritative legacy input until their authors
upgrade them; local editing of synced rows stays disabled. An invalid version-2
file follows existing freeze-and-report sync behavior.

Built-in YAML declares the field explicitly for every step. Preserve each
template's old behavior: qualifying final steps are true; other steps are false.
Do not make the final PR step in Improve Kandev terminal merely because it is
last. Update stored template definitions and workspace bootstrap paths too.

## Migration

The workflow repository owns the semantic backfill, after required additive
schema changes and before serving requests or starting sync. Task repository
projection/bootstrap initialization adds the same column before its own writers
need it. It does not independently infer or backfill completion.

Use one transaction and a durable `kandev_meta` key,
`workflow_complete_task_on_enter_backfill_v1`. Inside the transaction, read the
marker, backfill legacy steps and stored template JSON, then record completion.
Use the existing portable Go normalization for trimmed case-insensitive names
and final position. A final step has no sibling with a greater position,
matching `GetNextStepByPosition`; tied maximum positions retain that definition.
Preserve row timestamps and IDs. The transaction rolls back the marker and all
backfill writes on failure. Startup fails if this required migration fails.

Replay skips the semantic backfill after the marker. It must not use
`WHERE complete_task_on_enter = 0` as a recurring migration. Fresh startup writes
explicit built-in values and records the marker before ordinary user writes.
Template backfill preserves unrelated JSON fields and respects explicit values.
Required-store conformance covers actual constructor order, fresh SQLite and
PostgreSQL, old-schema upgrade, replay after disabling, and failure/retry.
Historical release fixtures remain truthful snapshots; add assertions or a
dedicated legacy fixture rather than editing old releases to contain new columns.

## Entry and notification

Replace the name test in `IsTerminalStep` with the explicit boolean while
retaining final-position eligibility. A step completes work only when it has
no successor and its setting is true.
Keep the old name predicate private to legacy conversion/migration. Audit all
callers, including task creation, `task/service.service_workflow`,
`orchestrator.workflow_store`, queued moves, automated transitions, and
`task_launch_workflow`. Every caller must resolve the actual successor or
equivalent final-position evidence; passing nil without a lookup is not proof.

Persist step and task-state changes through existing guarded transition and
ledger operations. A checked final entry completes a nonterminal task; an
unchecked or non-final entry follows existing nonterminal/reopen behavior.
Do not convert FAILED or
CANCELLED to success. Saving the checkbox does not reevaluate resident tasks.
Do not add transition actions or replay on-enter during Resume.

Parent rollups and dependencies use committed task state. Remove name- or
step-membership fallback from `annotateTerminalChildSteps` and terminal receipt
handling. Preserve `DependencyStatusForTask`'s existing state-only resolution.
Completion events follow the
successful state write, never a speculative target lookup. Preserve existing
operation deduplication, failure/cancellation rollups, archived-child filtering,
and success-only dependency launches. Upgrade does not rewrite resident tasks
or fire old completion events; pre-existing persisted completion stays intact.

## Explicit resume

Reuse `session.recover` with `action: resume`. Admit completed recovery only
through this explicit operation or a session-pinned follow-up send. Carry a
narrow internal completed-resume permission through `LaunchSession` and
`executor.ResumeOptions`; automatic `session.ensure`, status reads, generic
open-time launch, and startup recovery never infer that permission.

Authorize the task/session pair and prompt access before side effects. Under
the existing lifecycle/cancel guards, reload the exact session and task, verify
archive and runtime ownership, and claim the current completed state/version.
Require the expected execution generation as well as state where a callback
can replace the session during recovery. Do not weaken global terminal guards.

Extend `allowsSessionStartingRecovery` only for an explicitly claimed completed
resume. Permit absent `executors_running` after cleanup; retain recoverable task
environment and profile checks. Reuse stale-execution cleanup, provider token
restoration, and native/CLI/history fallback. Persist `STARTING` and clear
`CompletedAt` through the guarded writer before credentials are issued. Retain
prior execution and turn history; do not clear ACP/provider identity.

At readiness, settle into the existing input-capable state without sending a
synthetic workflow or task-description prompt. Preserve current initial-message
backfill only for genuinely absent launch history. The next user send starts
one new conversational turn after the old one. No new session ID or queue
incarnation is allocated by Resume.

`message_target_session.go` continues to prefer live primary, then live
fallback. Completed siblings must not shadow a live session. With no live
session, a completed primary, or the newest completed session if no primary
exists, can be selected and pinned. An explicit completed `session_id` always
targets that session. Authorization and validation precede the same resume
admission, which runs before `ProcessOnTurnStart` and message persistence.
FAILED/CANCELLED dispatch continues to reject with its existing error code.

## Follow-up ownership

A message on a completed task is conversation work. Runtime running, waiting,
failed, and finished events do not reopen that task or replay workflow entry,
turn-start, turn-complete, completion-signal, or parent notification effects.
Existing explicit task moves/state APIs remain the way to reopen workflow work.

For an explicitly resumed completed session on a still-active task, persist a
session metadata marker `completion_follow_up` in the guarded resume claim.
It records conversational-only ownership independently of the mutable session
state. Such a session stays excluded from automatic profile reuse and cannot
become primary merely because it resumes. Automatic workflow triggers and
runtime task-state reconciliation skip it. An explicit workflow handoff may
clear the marker only while establishing that session's workflow ownership.
An ordinary send, page activation, or automatic reuse lookup cannot clear it.

Pinned user and MCP sends, queued dispatch, streaming completion, agent exit,
and failure publication consume the same ownership predicate. This protects a
retired profile conversation even after its state becomes `WAITING_FOR_INPUT`.
Preserve primary IDs, profile routing, step history, and current queue policy.

## Cleanup and races

For successful task completion, stop collapsing an ordinary child conversation
to `COMPLETED` in `setSessionWaitingForInputIfRequestedWithHook`. Settle it with
the existing root-task waiting behavior. Keep task status completed and retain
the fail-closed idle resource reclamation. Do not change failed/cancelled task
receipt rules through this branch.

Use current-turn clarification lookup; any active clarification retains its
existing barrier and late-answer path. Historical answered or expired bundles
remain history. A lookup error cannot justify closing the conversation.

Cleanup and resumed execution must use the same lifecycle ownership boundary.
An old receipt, stop callback, stream frame, or reclaim operation carries its
execution identity; after a newer resume claim it cannot clear queues or stop
the replacement runtime. Preserve retired-execution tombstones. Release guards
before waiting for callbacks that need those guards.

Queue admission and readiness use the existing queue-incarnation and dispatch
reservation contract. Never clear pending messages solely because the old
execution completed. Duplicate Resume joins or returns the existing attempt.
A later stop/archive/delete wins over stale recovery writes. A failed launch
uses existing recoverable error state without changing completed task state;
retain the follow-up marker for retry and preserve queued/unsent content.

## User interface

`SessionStoppedBanner` completed mode has primary Resume and secondary New
Agent. Reuse `useSessionRecoveryActions` and existing recovery error feedback.
Show busy state, disable duplicate actions, and restore the composer from the
authoritative session update. Missing profile feedback stays visible. New
Agent retains its dialog and intentional new-conversation behavior.

Keep the completed-chat open-time gate in session-resumption hooks. Extend
recovery hook tests for response success/failure, session switching while a
request is pending, and busy cleanup. Do not let a late response modify another
chat's recovery feedback or selected session.

Show the completion checkbox only in the final step's entry/general settings,
outside the conditional turn-complete controls. It remains available when the
final step has no actions. Put the description inside an adjacent info icon
using the existing HelpTip interaction: hover or focus on desktop, tap on touch.
Do not render the description permanently below the checkbox. Suggested text:
"Marks the task complete when it enters this final step. You can still continue
the conversation." Give the icon a localized accessible name. Use coordinated
dirty/save/discard state and read-only sync treatment. Localize all copy.

### Mobile composition

The existing `task-layout.tsx` phone layout and inline stopped-session banner
are the chat exemplar. Keep the transcript's single scroll owner and the
composer area's safe-area spacing. Stack Resume then New Agent on phones;
retain compact side-by-side buttons on fine-pointer desktop. Phone/coarse
pointer actions have at least 44 px active height; no unconditional desktop
`min-h-11` sizing is introduced.

The step editor's `CancelCompletionToggle` supplies the checkbox/label pattern;
the existing mobile workflow step selector supplies focused navigation. Use a
44 px tappable label and a touch-accessible info icon. Disclose wrapping helper
text through the existing touch help surface; do not require hover or add a
new standalone modal. Opening help must not toggle the checkbox.
An inline setting fits this short choice. Both viewports share draft and save
logic. Preserve keyboard focus, one settings scroll region, no horizontal page
overflow, and access to Save changes above safe-area insets.

## Verification and observability

Use barrier-based Go tests for resume/stop, reclaim/resume, stale receipt/new
execution, double Resume, and readiness/queue admission. Assert the preserved
task, primary session, provider token, history, queue order, and emitted events.
Test both database dialects at migration boundaries. Use existing structured
logs with task/session/execution identity and resume outcome; do not log tokens.

Desktop and mobile Playwright tests save/reload the final-step checkbox, complete
checked final steps, send from unchecked final steps, and Resume a persisted
completed chat. Reorder tests prove the control moves to the current final
step and a previously checked non-final step cannot complete work. Help tests
prove hover/focus/tap disclosure and that opening help does not toggle the value.
Assert a second agent response, unchanged session count and history, and no
task reopen or duplicate completion. Include a retired non-primary session and
active clarification regression. Run against freshly built production assets.

## Related decisions

- [Separate task completion from conversation availability](../../../decisions/2026-09-09-task-completion-conversation-availability.md)
- [Replayable migrations](../../../decisions/0027-replayable-schema-migrations.md)
- [Profile-switch session policy](../../../decisions/2026-08-31-workflow-profile-session-switch-policy.md)
- [Current-turn clarification ownership](../../../decisions/2026-08-14-current-turn-clarification-ownership.md)
