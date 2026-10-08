---
status: current
system: ui
requirements:
  - REQ-UI-REVIEW-COMMENT-DELIVERY-001
---

# Review comment delivery system design

## Purpose and boundaries

This design corrects acknowledgement and removal of browser-held feedback in
the shared Review send path. The [requirement](../requirements/review-comment-delivery.md)
extends the local lifecycle owned by [review file comments](review-file-comments.md).
The original file-comment design described fire-and-forget loss and excluded
repairing it. This package repairs that separately scoped contract;
it does not change task message admission, queuing, or provider-hosted reviews.

Ownership discovery also inspected Tasks' comment-draft preservation and plan
comments: those govern task-comment textarea and task-owned plan data, not the
local file/line Review store. No system boundary change or ADR is needed for
this local correction. The requirement, design, and work order preserve its
constraints sufficiently; no generic draft or delivery framework is introduced.

## Requirement mapping

| Criteria under REQ-UI-REVIEW-COMMENT-DELIVERY-001 | Design section |
| --- | --- |
| `.1`, `.2`, `.3`, `.6`, `.9` | Admission and acknowledgement |
| `.4`, `.5` | Snapshot and settlement |
| `.7`, `.8` | Dialog ownership and surfaces |

## Original causal path

- `ReviewTopBar.handleFixComments` calls its synchronous `onSendComments` and
  then `markCommentsSent` immediately.
- `ReviewDialog`'s `useReviewDialogHandlers.handleSendComments` invokes the
  callback and closes immediately.
- `useReviewDialog.handleReviewSendComments` starts `message.add`, catches
  rejection only to toast, and closes without awaiting a response. Missing
  client returns silently.
- `comments-store.ts`'s `markSentInState` deletes supplied IDs from `byId`,
  `bySession`, and `pendingForChat`, then persists each affected session.
  This action remains valid for other consumers and is not changed globally.

ROOT's read-only proof establishes two actual-store losses and one success
control at `25d62239e990add8fe313722022f6476f4bf4c5a`. It composes the actual
TopBar and hook with actual stores/providers, not the full ReviewDialog.
Persistence deletion is source-confirmed; the failing store assertions preceded
the proof's persistence assertions. Earlier missing-provider fixtures and the
transport-null rerender warning are not additional product findings.

## Components and contracts

| Boundary | Responsibility |
| --- | --- |
| `components/task/use-review-dialog.ts` | Own local admission, captured task/session and submitted rows, awaited transport, selective store settlement, scoped automatic close, and pending flag |
| `components/task/dockview-review-dialog.tsx` | Pass the real asynchronous callback and pending flag into ReviewDialog |
| `components/review/review-dialog.tsx` | Declare `onSendComments: (comments: ReviewComment[]) => Promise<boolean>` and optional pending prop; return the callback's promise through the handler without closing |
| `components/review/review-dialog-surface.tsx` | Forward pending and asynchronous handler into ReviewTopBar; remove the premature-clear callback wiring |
| `components/review/review-top-bar.tsx` | Await the handler; do not delete comments or close on invocation; forward pending into FixCommentsButton |
| `components/review/review-fix-comments-button.tsx` | Disable the existing button while pending, retaining its label, overview, and touch activation |

`true` means the admitted transport request was acknowledged; `false` means no
acknowledgement was obtained or no attempt was admitted. All production
`onSendComments` callers were audited: TaskReviewDialogMount is the sole
ReviewDialog production caller. It serves desktop and tablet layouts and is
wrapped by `SessionMobileReviewDialog` on phones. Test callbacks must remain
type-correct without replacing this contract with implicit synchronous success.

## Admission and acknowledgement

Keep a synchronous pending ref in the mounted `useReviewDialog` owner plus
rendered pending state. Set admission before starting the request; a second
activation returns false without a request or comment removal. This ref
survives the shared dialog's dismissal/reopening while the hook stays mounted.
There is no module/global latch, cross-composer lease, or automatic retry.

Empty selection, missing task/session, or an already pending owner cannot
report success. An absent or disconnected client preserves notes and uses the
existing localized `task:failedToSendComments` error toast. Check the existing
client's `getStatus()` before admission: the transport queues offline requests
and starts their timeout only after connected dispatch, which would otherwise
hold Review pending indefinitely and flush feedback on reconnect without a
new deliberate attempt. This local guard does not change transport queuing,
reconnection or timeout semantics for any consumer. Await the existing request:

```text
message.add
{ task_id: capturedTask, session_id: capturedSession,
  client_message_id: generateUUID(), content: submittedMarkdown }
timeout: 10000
```

Keep current Markdown formatting, generated message IDs and transport timeout.
`WebSocketClient.handleRequestResult` resolves correlated `response` frames and
rejects `error` frames. Only resolution authorizes settlement. Catch rejected
or synchronously thrown transport errors, preserve the current store, show the
existing localized failure feedback, and return false. Clear local pending in
the request's finally boundary. Do not treat a timeout as server rollback or
add a replay, reconciliation protocol, or exactly-once claim.

## Snapshot and settlement

At admission capture task/session and a fresh submitted array containing the
actual immutable store row references. `getPendingComments` in ReviewDialog
filters by session and `isReviewComment` without cloning rows. The Zustand
Immer store preserves unchanged row references and replaces changed rows,
including `updateComment` text or metadata edits. The request is formatted
before awaiting; subsequent edits cannot change that body.

After acknowledgement, reread the actual CommentsStore. A submitted row is
eligible to remove only if its ID still points to that exact submitted row,
it still belongs to the captured session, remains pending, and remains in the
pending set. Pass only those IDs to the existing `markCommentsSent` action.
The check and action execute synchronously in settlement, without an intervening
await. No supplied ID alone is authority to delete a changed row. Conservatively
preserve any replaced row, including edit-away-and-restore; there is no new
revision field or storage migration.

This local rule protects the full `ReviewComment` discriminated union: line
anchors, side, code excerpt, file/repository identity, and optional whole-file
`baseRef`/`isSubmodule` metadata as well as exact raw text. New same-session
notes, another session's notes, plan/file-editor/other comment sources, and
explicit deletions are untouched. Do not restore an old snapshot on failure.

## Dialog ownership and surfaces

Remove both invocation-time close calls from the dialog wrapper and task hook.
After acknowledged selective settlement, close from the task hook only if its
current displayed task/session still matches the captured owner and there are
no pending review notes left for that session. Use a local current-owner ref
to guard a late result when the mounted hook's inputs change; do not build a
navigation coordinator. Unrelated comment sources do not prevent the ordinary
successful close. Settlement never opens a dialog. Preserve user Close,
Escape/outside dismissal, and existing no-files auto-close policy.

The shipped mobile exemplar is `SessionMobileReviewDialog` wrapping the same
TaskReviewDialogMount. The curated dense-content-viewer baseline informs the
existing focused Review body; this correction does not replace its geometry.
Desktop retains the file sidebar and diff body, phone retains its sidebar-free
shared surface and tap-to-overview activation. No CSS, breakpoints, safe-area,
scroll, layout, touch sizing, label, or navigation redesign is included.

The mobile-parity state/data exception applies narrowly to admission and
settlement inside existing controls. Pending only sets the existing button's
disabled/busy semantics with stable copy, without introducing a spinner, status
label, overlay, or new interaction. Real rendered tests of both actual mounts,
including coarse-pointer first tap to overview and second tap to send, supply
parity evidence. No new Playwright/browser/build run is planned. If implementation
requires presentation changes beyond this boundary, checkpoint ROOT before
expanding resources or verification.

## Verification and persistence

The independently authored `components/task/dockview-review-dialog.delivery.test.tsx`
renders actual TaskReviewDialogMount and actual
SessionMobileReviewDialog with real ReviewDialog, TopBar, FixCommentsButton,
StateProvider/createAppStore, ToastProvider, TooltipProvider, VcsDialogsProvider,
and any further required production providers. Seed actual session/environment,
repository and ready diff state; explicitly answer auxiliary WS/fetch reads.
Mock only external WS/fetch transport. Do not mock hooks, formatter, store,
components, or internal send adapters and do not import/copy/replay ROOT's proof.

Deferred transport gives exact pending and settlement boundaries without sleeps.
Verify real controls, store indexes and `loadSessionComments` separately so
persistence evidence is actually reached. Assert the exact wire task/session,
UUID-shaped message ID and fixed independently authored expected Markdown;
IDs and all metadata must remain exact in retained/persisted notes. Preserve
the acknowledgement-success control alongside causal rejection/missing-client
RED. Scope queries to the active dialog and settle/join all owned requests and
timers in cleanup. The [work order](../../../plans/preserve-review-comments/task-01-confirm-review-delivery.md)
maps all criteria to scenarios and exact bounded commands.

Persistence retains `kandev.comments.<sessionId>` sessionStorage, existing
hydration, and storage-availability limitations. No new storage tier, schemas,
backend changes, logs of feedback, or metrics are needed.

## Public documentation reconciliation

The docs-maintainer audit searched `docs/public/**`, root README and screenshot
catalog plus adjacent specs/decisions. `docs/public/sessions-and-review.md`
replaces the original loss warning with confirmed retention, deliberate retry,
remaining-edit behavior, and uncertainty limits. Its same-tab storage and
stale-anchor guidance remain. The failure paragraph in the existing
review-file-comments design links here. These updates followed implemented
GREEN; they were deferred during design. No new public page,
screenshots, terminology, or AGENTS guidance is needed.

## Implementation plan

- [Preserve review comments](../../../plans/preserve-review-comments/plan.md)
