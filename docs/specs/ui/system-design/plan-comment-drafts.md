---
status: current
system: ui
requirements:
  - REQ-UI-PLAN-COMMENT-DRAFTS-001
---

# Plan Comment Drafts System Design

## Purpose and boundaries

This design keeps session-scoped pending plan comments safe while the shared
task Plan editor changes its visible comment projection. The UI system owns the
browser-local comment store, Tiptap marks, session-selection response, and
composer context. The task system continues to own plan content, revisions,
sessions, and message delivery.

No backend, API, WebSocket, task-plan schema, or comment data-model change is
required. `PlanComment.sessionId` remains the delivery and persistence owner.

## Root cause and evidence

`useTaskPlan(taskId)` correctly treats the plan document as task-scoped.
`TaskPlanPanel` separately calls `usePlanComments(activeSessionId)`, so selecting
a sibling Agent session replaces the editor's `comments` prop with that
session's projection.

Before this repair, `rehydrateCommentMarks` removed every Tiptap `commentMark`
absent from the new projection without recording why. That programmatic
document transaction reached the `CommentMark` orphan detector. The detector
interpreted the removed mark as a destructive edit and called
`onOrphanedComments`. `TaskPlanPanel.handleCommentDeleted` then removed the
comment from the Zustand store. `persistSessionComments` removed the owning
session's `sessionStorage` key when no comments remained, making the loss
permanent.

A focused temporary Vitest reproduction rendered one comment, rerendered the
same editor with an empty comment projection, and observed
`onCommentDeleted([commentId])`. It passed against the current code. The test
was removed after diagnosis; the implementation work order replaces it with a
permanent failing-first regression.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-PLAN-COMMENT-DRAFTS-001` | [State and persistence](#state-and-persistence), [Transaction provenance](#transaction-provenance), [Session-switch flow](#session-switch-flow), [Verification](#verification) |

## Components and responsibilities

- `useCommentsStore` remains the authoritative browser-local comment state and
  persists each session under `kandev.comments.<sessionId>`.
- `usePlanComments` selects only `PlanComment` records owned by the selected
  session and creates new comments for that session.
- `TaskPlanPanel` projects the selected session's comments into the shared Plan
  editor and clears transient selection/edit state when session identity
  changes.
- `TipTapPlanEditor` asks `rehydrateCommentMarks` to reconcile rendered marks
  with the supplied projection.
- `CommentMark` distinguishes projection-reconciliation transactions from
  user-authored document transactions before reporting orphaned comments.
- Task chat continues to include and clear only the active session's pending
  plan comments after successful delivery.

## State and persistence

The existing `CommentBase.sessionId`, `CommentsState.bySession`, and
`kandev.comments.<sessionId>` storage keys remain unchanged. Switching sessions
does not migrate or clone comments. The selected session controls which comment
IDs become editor marks and composer context.

An empty projection means no plan-comment marks are rendered for the selected
session. It does not mean comments belonging to another session were deleted.
Returning to the owning session reads its unchanged store entries and
rehydrates their marks from saved positions or selected-text fallback search.

## Transaction provenance

`rehydrateCommentMarks` shall tag the transaction it dispatches with a private
comment-projection reconciliation marker. The `CommentMark` orphan-detection
plugin shall ignore mark removals performed by tagged transactions.

Untagged user document transactions retain current orphan cleanup. For every
untagged document-changing transaction, the plugin compares comment IDs before
and after that transaction, then reports only IDs still absent from the final
document state. This keeps a real destructive edit authoritative without
mistaking projection changes for user intent.

Explicit deletion remains safe: the UI removes the rendered mark and store
entry as one user action, and any deferred orphan cleanup is idempotent. Tagged
additions and removals from projection changes never affect store state.

## Session-switch flow

1. The user adds a plan comment while session A is selected. Store and
   `sessionStorage` record session A as owner.
2. The user selects session B. `usePlanComments` publishes session B's comment
   projection.
3. Tiptap dispatches one tagged reconciliation transaction that removes A's
   visible marks and adds B's marks. Orphan cleanup ignores those presentation
   changes.
4. `TaskPlanPanel` closes any open comment editor and clears global
   `editingCommentId` plus local text-selection state for the old session.
5. Selecting session A publishes its unchanged comments. Tagged reconciliation
   restores their marks, badges, and editable feedback; task chat restores the
   same session's context chip.

## Failure and recovery

- If mark position metadata is stale after plan editing, existing selected-text
  fallback search remains responsible for finding the range.
- If a destructive user edit removes a mark, untagged orphan detection removes
  the associated pending comment as before.
- A failed direct, queued, or passthrough send leaves comment state unchanged.
- A session switch during an open editor closes the editor rather than applying
  its draft to another session.
- Comments already deleted by the old path have no backend copy and cannot be
  reconstructed by this repair.

## Responsive behavior

Desktop and mobile reuse the same comment store, Plan editor, mark extension,
and session identity. The repair changes no composition, overlay geometry,
scroll owner, safe-area handling, or touch target. Existing desktop Popover and
mobile Drawer presentation remain unchanged.

Because this is state and transaction normalization inside the shared editor,
focused unit coverage plus one desktop multi-session E2E flow provides the
cross-viewport logic evidence. No new mobile Playwright scenario is required;
existing mobile Plan comment coverage continues to own touch presentation.

## Verification

- `tiptap-plan-editor.test.tsx` changes the `comments` projection and proves the
  previous comment is visually unmounted without invoking deletion. A paired
  case performs a real destructive editor transaction and proves orphan cleanup
  still fires.
- A focused `TaskPlanPanel` test changes active session identity while a comment
  editor is open and proves transient editing state closes without mutating the
  old comment.
- `multi-session-ux.spec.ts` creates a plan and two sessions, adds a pending
  comment in the primary session, selects the sibling, returns to the primary,
  and proves the badge and feedback text are restored.

No production telemetry is added. The failure is deterministic and covered at
the transaction, component, and user-flow boundaries.

## Alternatives considered

- Keying Tiptap editor instances by session would avoid the callback by
  destroying the editor, but it would add expensive remounts and focus/selection
  churn to a task-scoped document.
- Guarding deletion only in `TaskPlanPanel` would leave transaction provenance
  ambiguous and remain vulnerable to deferred callback races.
- Making plan comments task-scoped would change which Agent receives pending
  prompt context and is outside this repair.

## Related decisions

None. The design separates programmatic reconciliation from existing user
intent without creating a new public or architectural boundary.
