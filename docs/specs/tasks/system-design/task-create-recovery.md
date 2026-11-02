---
status: current
system: tasks
created: 2026-10-27
requirements:
  - REQ-TASKS-CREATE-RECOVERY-001
owners:
  - kandev
---

# Task creation recovery system design

## Purpose and boundaries

The native creation dialog becomes a caller of the shipped REST external-ID contract.
The browser owns a submission receipt. The backend owns task identity and creation settlement.
The browser never takes ownership of an incomplete server creation.

The [requirement](../requirements/task-create-recovery.md) owns observable outcomes.
The [external-ID design](external-id-idempotency.md#rest--create) owns existing create and lookup responses.
This package uses its shipped uniqueness and settlement behavior.
It does not implement that document's separate draft aggregate coordinator or recovery worker.

The [Quick Chat acceptance ADR](../../../decisions/2026-10-26-quick-chat-creation-acceptance.md)
provides a receipt precedent. Its endpoint-specific resume authority remains exclusive to Quick Chat.
Normal task creation does not gain that exception.

## Requirement mapping

| Criteria under `REQ-TASKS-CREATE-RECOVERY-001` | Design section |
| --- | --- |
| .1, .2, .3, .4 | Submission identity and captured payload |
| .5, .6 | REST contract and rejection evidence |
| .7, .8, .10, .11 | Receipt persistence and ownership |
| .9, .14 | Acceptance and existing creation modes |
| .12, .13 | Recovery surface |

## Current source boundaries

| Component | Responsibility |
| --- | --- |
| `apps/web/components/task-create-dialog-submit.tsx` | Native submit entry points and acceptance side effects. |
| `apps/web/components/task-create-dialog-helpers.ts` | `buildCreateTaskPayload`. |
| `apps/web/components/task-create-dialog-fresh-branch-consent.ts` | Consent and its one explicit re-consent retry. |
| `apps/web/components/task-create-dialog-state.ts` | `useDraftPersistence`, current form, and successful draft cleanup. |
| `apps/web/lib/local-storage.ts` | Per-workspace sessionStorage draft helpers. |
| `apps/web/lib/api/domains/kanban-api.ts` | `createTask` and `fetchTask`. |
| `apps/backend/internal/task/handlers/task_http_handlers.go` | `httpCreateTask`, found responses, and external-ID lookup. |
| `apps/backend/internal/task/service/preallocation_error.go` | Existing `PreAllocationTaskCreateError` evidence. |

Add a small receipt module and a dialog recovery hook.
Keep the hook separate from the large submit module.
Do not turn Quick Chat's endpoint-specific hook into a generic creation framework.

## Submission identity and captured payload

Before the first await, one shared ref guard admits a submit action.
Local validation and required branch consent run inside that guard.
Only a valid, consented submission receives a UUID.

The browser sends `external_id: ui-task-create/<uuid>` with the native REST payload.
This is a caller-generated external identity, not a backend-generated default.
It uses the ordinary namespace and requires no reserved-prefix privilege.
The server continues to accept legacy requests without this field.

A versioned receipt contains:

| Field | Meaning |
| --- | --- |
| `version`, `submissionId`, `externalId` | Decoder version and immutable identity. |
| `userId`, `workspaceId`, `draftId` | Browser owner, workspace, and draft binding. |
| `payload` | Captured normalized creation values, including original start intent. |
| `draftTitle` | The form title at submission, including an auto-generated title omitted from the request. |
| `state` | `submitting`, `unknown`, `incomplete`, `accepted`, or `unavailable`. |
| `everUncertain` | Sticky evidence that an earlier attempt can still finish. |
| `taskId`, `creationComplete` | Exact observed server identity and settlement. |
| `replayable` | Whether every required payload value remains available. |
| `runtimeId`, `handoffAttempted` | Runtime ownership and a fence against repeated acceptance callbacks. |

The payload includes workflow, parent, repositories, profiles, executor, options,
dependency intent, MCP selections, text, and ready attachment descriptors.
The accepted action fixes `start_agent`, `prepare_session`, and `plan_mode`.
Retry never rebuilds these values from the current form or last-used settings.

The existing fresh-branch helper can update only explicit renewed discard consent.
Both calls keep the same external ID and all other submitted values.
A found incomplete result ends that sequence without another branch operation.
An untyped error or uncertain rollback never permits a new identity.

The synchronous guard covers submit, create-only, plan-mode, and composer actions.
It also covers recovery actions, so Check status and Retry cannot race each other.
Independent confirmed submissions receive independent UUIDs even with identical text.

## REST contract and rejection evidence

### Native response decoding

Add native client support for the existing optional `external_id` request field.
Decode required `deduplicated` and `creation_complete` booleans at this transport boundary.
Keep `creation_complete` out of the general task model.
Custom transport response types need not acquire native-only requirements.

| Observation | Browser action |
| --- | --- |
| Created, complete | Record the task, then perform guarded acceptance. |
| Found, complete | Record the same task, then hydrate missing navigation data. |
| Found, incomplete | Retain the receipt and show the existing task. Do not prepare or start it. |
| Created, identity lost | Retain the returned task ID. Do not reuse the released key. |
| Foreign workspace, external ID, or task ID | Mark unknown; never adopt the response's task. |
| Malformed success, failed body read, timeout, or untyped error | Mark unknown and retain the receipt. |

Check status calls the existing read-only route:

`GET /api/v1/workspaces/:id/tasks/by-external-id?external_id=<encoded-value>`.

Validate the returned workspace, external ID, task ID, and settlement field.
Both workspace and external ID are required; a response that omits either is a mismatch.
Once observed, `taskId` cannot change within a receipt, and a previously verified ID survives later malformed data.
A lookup miss or read failure preserves the last observed accepted, incomplete, or unavailable state;
only a submission without an observed task becomes unknown.
A mismatched response leaves recovery unresolved and never selects a similar task.
Do not resolve through workflow snapshots, titles, profiles, repositories, or recency.

An explicit Retry first performs this lookup.
A match follows the table and does not send another create POST.
Only a lookup miss with no previously observed task ID permits a same-key POST.
A failed or unauthorized lookup does not establish absence.
Reconnect and reload can perform a read, but they never perform that POST automatically.

Found responses use `dto.FromTask` and can omit `session_id` and primary-session fields.
Use `fetchTask(taskId)` when navigation requires that existing session information.
Failure to hydrate it retains the known task and offers Open task.
It never becomes a reason to allocate or launch again.

### Definitive rejection

Add optional `creation_rejected: true` to native REST error bodies only with proof
that this request rejected the submission before task allocation.
Preserve existing statuses, `error_code`, dirty-file details, and policy error fields.
Do not modify Quick Chat's separate `quick_chat_creation_rejected` contract.

Safe evidence includes handler validation before `Service.CreateTask` and typed
`PreAllocationTaskCreateError` for an allowlisted validation or reference failure.
A stale branch policy is one allowlisted rejection: its `branch_policy_stale` code is
preserved, the browser refreshes policy choices, and only a first unresolved attempt retires.
A typed repository-provider unavailable reference failure is also pre-allocation, so its
existing 503 status and `repository_selection_unavailable` code carry the marker.
Use the existing marker rather than inferring allocation from an error message.
Generic 4xx, database errors, request cancellation, and post-create failures carry no marker.
An attachment, fresh-branch, session-prepare, or settlement error after allocation is not a rejection.
The marker is absent when the handler cannot prove the boundary.

For a first attempt without uncertainty or an observed task, the marker proves rejection.
The browser can retire that receipt and keep the draft editable.
The corrected explicit submission receives a fresh identity.

After any uncertain attempt, `everUncertain` remains true.
A later request's pre-allocation rejection proves nothing about the earlier request.
That earlier request can still allocate after the rejection or a lookup miss.
The browser therefore keeps the original identity and snapshot in this race.
No ordinary status code, elapsed timeout, or repeated miss clears this latch.

## Receipt persistence and ownership

Persist a receipt before its first POST, using sessionStorage like the existing native draft.
Scope records by authenticated user, workspace, and draft ID.
Use a draft pointer to restore the matching receipt before enabling create actions.
Keep existing title/description-only draft records readable without migration.
Prefilled native dialogs also need a receipt even when ordinary draft saving is skipped.

An in-memory owner retains active receipts during close transitions and storage errors.
Storage helpers must report failures for this operation instead of silently discarding its receipt.
After a storage failure, the mounted owner keeps its guard and recovery actions.
The UI explains that recovery cannot survive reload until storage succeeds.
The guarantee does not extend to data the browser refuses to store.

The receipt stores submitted descriptors, not base64 data, authorization headers, or credentials.
Existing inline data can remain in memory for an exact retry.
After reload without those bytes, set `replayable: false` and allow lookup only.
Never drop the attachment and submit a changed payload.
Do not release or re-upload attachments merely because the dialog closes.

Owner generations fence user, workspace, draft, open-cycle, and submission changes.
Late responses can update only their own receipt through a compare-by-ID write.
They cannot close, navigate, or clear a different current form.
Acceptance clears only draft values and descriptors that still match the submitted snapshot.
Newer edits remain in the form and its persisted draft.

Close preserves unresolved receipts and does not mean server cancellation.
An accepted result received while closed remains available on reopen.
A visibly acknowledged accepted receipt can retire before a new intentional submission.
Retirement never calls the server's external-ID release route.

Known archived tasks remain the matching task. Recovery does not unarchive them.
Known deleted or inaccessible tasks enter `unavailable`, with no create POST fallback.
Observed explicit deletion retires that submission instead of resubmitting it.
Generic REST idempotency lasts only while the task holds its external identity.
Unobserved deletion or operator release has no server tombstone in this package.

## Acceptance and existing creation modes

Record `taskId` and accepted creation state before invoking `onSuccess`, clearing a draft,
recording last-used settings, changing plan mode, or navigating.
Keep presentation errors separate from creation errors after that point.
An Open task action uses the recorded task and the existing navigation path.
When the live draft differs from the submitted snapshot, including an auto-generated title,
description, or attachment identity, acceptance records the task but does not run callbacks,
clear the draft, close, or navigate.
Opening a task whose creation is not accepted adds a `creation=pending` route marker; the task page
then suppresses automatic session ensure so inspection cannot race the original create owner.
Explicit user start controls remain available.

Only the current owner can run automatic acceptance effects.
Run creation callbacks once per owner receipt, with the original action metadata.
Persist `handoffAttempted` before entering those callbacks, including callbacks that throw.
After reload or a previously completed callback, recovery opens the task without replaying callbacks.
When the current draft differs, preserve it and offer Open task without closing the dialog.
A later explicit new submission receives a fresh key.

| Native action | Accepted behavior |
| --- | --- |
| Start task | Preserve backend-owned initial prompt delivery and automatic focus preference. |
| Plan mode | Resolve the existing prepared session before the existing plan-mode navigation. |
| Create only | Accept a complete task without requiring a session or launching one. |
| Dependency or WIP queue | Preserve queue notification and deferred-start intent. Session absence is not failed creation. |
| Native prefilled task or subtask | Capture its parent/context and preserve completion callbacks. |
| Custom `createTask` override | Preserve its current adapter contract. Do not inject native idempotency or lookup. |
| Edit task, additional session, Quick Chat | Bypass this native creation owner. |

The normal dialog does not submit an extra chat message during recovery.
Existing launch failures remain the responsibility of
[task launch failure recovery](task-launch-failure-recovery.md).

## Recovery surface

Reuse the standard TaskCreateDialog surface and its existing internal scroll body.
The closest mobile precedent is its full-height Create Task composition, including
`mobile-task-create-escape.spec.ts` and the visible phone footer alternatives.
This flow contains a full draft and several choices, so it remains a full-height phone surface.
It does not add a nested sheet or change the surrounding navigation.

Add one inline status region above the footer.
Keep draft controls usable for later edits, but replace ordinary submit actions
while the earlier submission remains unresolved.
The status explains that retry uses the earlier values and keeps newer edits.

| State | Primary action | Other actions |
| --- | --- | --- |
| Unknown, no task ID | Check status | Retry submission when replayable, Close |
| Incomplete, known task | Open task | Check status, Close |
| Accepted, presentation incomplete or newer draft | Open task | Close, or an explicit fresh submit after resolution |
| Known task unavailable | Check status | Close |
| Read or POST pending | Disabled progress action | Close preserves the receipt |

Use existing Button and status primitives with `t()` and seven-language catalogs.
Use a polite status announcement and semantic action names.
On fine-pointer desktop, keep normal 28px controls.
On phones, use stacked footer actions with at least 44px measured targets.
Use the existing safe-area treatment, dynamic viewport, and a single scrolling form body.
Escape keeps its existing create-dialog behavior.
No recovery action depends on hover, overflow menus, or keyboard shortcuts.

## Observability and verification

Use the existing bounded frontend diagnostic logger for state changes and receipt outcomes.
Record state, outcome, and correlated task ID without prompts, bodies, inline data, or credentials.
Keep submission IDs out of metric labels. Add no polling metric or permanent debug instrumentation.

Unit tests cover receipt identity, exact retries, owner fences, sticky uncertainty,
storage failure, descriptor persistence, malformed responses, and acceptance side effects.
Handler tests prove the rejection marker's allocation boundary and unchanged found outcomes.
Desktop and phone E2E forward the first POST to the real isolated backend.
They replace its browser response with a transport error, then recover the retained task.
The assertion counts backend tasks and sessions, not just intercepted requests.

## Implementation plan

[Task creation retry protection](../../../plans/task-create-recovery/plan.md).
