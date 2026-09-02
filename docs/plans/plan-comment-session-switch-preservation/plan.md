---
created: 2026-09-02
status: draft
requirements:
  - REQ-UI-PLAN-COMMENT-DRAFTS-001
system_design:
  - ../../specs/ui/system-design/plan-comment-drafts.md
legacy_specs: []
---

# Implementation Plan: Preserve Plan Comments Across Session Switches

## Overview

Prevent Agent-session navigation from deleting another session's pending Plan
comments. Implement transaction provenance at the Tiptap mark boundary, reset
stale editor state at the session boundary, then prove the complete primary to
sibling to primary flow.

## Confirmed root cause

`TaskPlanPanel` supplies session A's comments to a task-scoped Tiptap editor.
Selecting session B supplies a different projection. `rehydrateCommentMarks`
programmatically removes A's marks, but `CommentMark` reports that removal as a
user-created orphan. `TaskPlanPanel` responds by deleting A's comment from the
store and `sessionStorage`.

A temporary focused Vitest reproduction passed before planning: rerendering
`TipTapPlanEditor` from one comment to an empty projection emitted
`onCommentDeleted` with the original ID. The temporary test was removed after
capturing the evidence.

## Scope

### In scope

- Distinguish projection reconciliation from destructive user edits.
- Preserve session-scoped comment state and storage across session switches.
- Close stale comment editing/selection state when active session changes.
- Restore marks, badges, feedback, and composer context when returning to the
  owning session.
- Add transaction, component, and desktop multi-session regression coverage.

### Out of scope

- Backend or WebSocket changes.
- Task-scoped or server-persisted pending comments.
- Plan editor, Popover, Drawer, or navigation redesign.
- Recovery of comments already removed by released versions.

## Technical approach

### Mark reconciliation

Update `comment-mark.tsx` so `rehydrateCommentMarks` tags its own mark-only
transaction. Orphan detection skips tagged projection removals and continues to
report IDs removed by untagged user document changes. Before reporting, confirm
the ID remains absent from final document state.

### Session boundary

Update `TaskPlanPanel` selection handling so a changed `activeSessionId` closes
the open comment editor, clears editor/browser selection, and clears
`editingCommentId`. Keep `usePlanComments` and comment persistence keyed by
session.

### Regression coverage

Add failing-first unit/component tests for projection replacement, real orphan
cleanup, and stale edit-state reset. Extend `multi-session-ux.spec.ts` with one
primary to sibling to primary scenario that restores a pending comment's badge
and editable feedback.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-UI-PLAN-COMMENT-DRAFTS-001.1` | Existing add/persistence coverage plus the multi-session E2E setup |
| `AC-UI-PLAN-COMMENT-DRAFTS-001.2` | Tiptap projection unit regression and multi-session E2E |
| `AC-UI-PLAN-COMMENT-DRAFTS-001.3` | Multi-session E2E restores badge and feedback |
| `AC-UI-PLAN-COMMENT-DRAFTS-001.4` | Paired tagged-reconciliation and untagged-destructive-edit unit tests |
| `AC-UI-PLAN-COMMENT-DRAFTS-001.5` | Focused TaskPlanPanel session-change test |
| `AC-UI-PLAN-COMMENT-DRAFTS-001.6` | Shared editor/state unit evidence; no viewport-specific production branch changes |

## E2E tests

Extend `apps/web/e2e/tests/session/multi-session-ux.spec.ts` with
`preserves pending plan comments across session switches` under the Chromium
project. It creates a plan and two sessions, adds feedback with **Add**, selects
the sibling session, returns to the primary session, and opens the restored
badge to verify exact feedback text.

No new mobile spec is planned because the repair changes shared state and
Tiptap transaction semantics only. Existing mobile Plan comment flows continue
to cover the unchanged Drawer/touch presentation.

## Work orders

- [ ] [Task 01: Preserve plan comments across session switches](task-01-preserve-plan-comments.md)

## Verification results

Pending implementation. Diagnostic evidence:

- `pnpm exec vitest run components/editors/tiptap/tiptap-plan-editor.test.tsx -t 'reports the previous session comment as deleted'` passed against current code, proving the destructive callback path.
- Temporary reproduction removed after the run.

## Risks

- Suppressing every orphan callback would leak comments after real plan edits;
  regression coverage must prove only tagged reconciliation is ignored.
- Deferred Tiptap transactions can race a session change; transaction metadata,
  not the current React projection, must identify the event source.
- Global `editingCommentId` can cross session boundaries unless transient edit
  state is reset with the session identity.
