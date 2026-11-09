---
created: 2026-10-28
status: implemented
requirements:
  - REQ-UI-TASK-CLEANUP-CONFIRMATION-001
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-DELETE-RECOVERY-001
  - REQ-TASKS-DELETE-RECOVERY-002
system_design:
  - ../../specs/ui/system-design/confirmation-warning-hierarchy.md
  - ../../specs/tasks/system-design/dirty-worktree-deletion.md
  - ../../specs/tasks/system-design/task-delete-recovery.md
legacy_specs: []
---

# Implementation Plan: Quick Chat dirty worktree deletion

## Overview

Make deletion of a repository-backed Quick Chat possible when its worktree has local changes, after the user explicitly selects discard. The reported incident's directory type was not established; this package fixes the independently confirmed dirty-worktree path and does not claim to diagnose other filesystem failures.

Quick Chat currently calls `deleteTaskAfterUserAction` with default options. Its preflight detects dirty worktrees, but the helper ignores `requires_discard_consent` and submits without discard consent. The backend rejects the request before mutation. Existing service tests prove both rejection without consent and successful cleanup with consent. Quick Chat's confirmation does not offer the required choice. Both the conversation close and retained-creation delete use this confirmation and the helper.

## Scope

### In scope

- Apply existing task-delete inspection, explicit consent, and recovery rules to both Quick Chat deletion entry points.
- Preserve clean and local setup-tab closure and existing navigation and error behavior.
- Cover dirty worktrees and phone and desktop interaction.

### Out of scope

- Changes to backend cleanup authorization, ownership audits, expiration, generic API caller behavior, or arbitrary nonempty scratch directory failures.
- Automatic discard, arbitrary filesystem deletion, or silent fallback to retain mode.

## Technical approach

Adapt `apps/web/components/quick-chat/quick-chat-delete-dialog.tsx` to use the existing task confirmation/preflight behavior, including clean, dirty, unavailable, and unknown inspection, explicit selection, retry and keep-files recovery. Reuse the existing confirmation controls and localized copy where appropriate. Resolve the actual task ID before opening the backed-chat confirmation; an unbacked local setup closes locally as before. Wire the target and consent-bound confirmation ID through `quick-chat-modal.tsx`, `use-quick-chat-modal.ts`, `use-quick-chat-close-actions.ts`, `quick-chat-creation-status.tsx`, and `use-quick-chat-creation.ts` to `deleteTask` with the exact selected options. Do not obtain a new preview behind the user's back or discard the preview result. Confirmations reset on close or target change, late preflights cannot authorize another target, and a dirty race or stale ticket keeps the chat and offers localized retry. Preserve success-only tab removal and creation-state cleanup. Do not widen the behavior of `deleteTaskAfterUserAction`, which has other callers.

The task deletion dialog is the closest mobile exemplar: a centered inset alert with a single internally scrolling body, persistent stacked actions and at least 44px phone/coarse-pointer targets. Desktop uses compact actions. Both entry points share state and semantics across widths. The task-dialog recovery choices remain available; selection never performs deletion by itself.

## ASCII UI preview

UI-01: Quick Chat delete, dirty worktree, desktop (AC-UI-TASK-CLEANUP-CONFIRMATION-001.12, .13)

```text
+--------------------------------------------------+
| Delete Quick Chat                                |
| Conversation and task will be deleted.           |
| Worktree and its files will be removed.           |
| [ ] Discard tracked and untracked local changes  |
|                                                  |
|                          [Cancel] [Delete]        |
+--------------------------------------------------+
```

UI-02: Quick Chat delete, dirty worktree, phone (AC-UI-TASK-CLEANUP-CONFIRMATION-001.14)

```text
+----------------------------+
| Delete Quick Chat          |  fixed heading
| Conversation and task      |  scrolling body
| will be deleted.           |
| Worktree will be removed.  |
| [ ] Discard local changes  |
|                            |
| [         Delete         ] |  fixed footer
| [         Cancel         ] |
+----------------------------+
```

The unchecked selection is required before Delete becomes enabled. Clean inspection hides it; loading disables Delete, failed inspection offers retry, and unknown inspection and keep-files follow the shared recovery contract. Copy and spacing are illustrative and localized; control order, explicit consent, scrolling, and touch reach are required.

## Tests

- Component and hook tests cover dirty, clean, unknown, failed/stale inspection, options bound to the target, cancel/reset, typed conflict, both entry points, and no-task setup closure (AC-UI-TASK-CLEANUP-CONFIRMATION-001.12-.14; AC-TASKS-DELETE-RECOVERY-001.6, .7; AC-TASKS-DELETE-RECOVERY-002.7).
- Existing backend `TestDeleteTaskRejectsDirtyWorktreeBeforeMutation`, `TestDeleteTaskWithDiscardConsentPersistsAndCleansDirtyWorktree`, and `TestTaskDeletePreflightUsesExactScopeAndReportsDirtyWorktrees` cover admission and cleanup (AC-TASKS-RUNTIME-CLEANUP-001.9).

## E2E tests

Add a `deletes dirty repository quick chat with consent` scenario to `apps/web/e2e/tests/chat/quick-chat.spec.ts` (`chromium`) and `apps/web/e2e/tests/chat/mobile-quick-chat-repository.spec.ts` (`mobile-chrome`). Create an isolated repository-backed chat and write an untracked marker to its actual task worktree; verify unchecked consent blocks deletion, cancellation preserves marker and chat, selection allows deletion, and the isolated worktree is eventually removed. Phone also verifies contained dialog and reachable touch controls. Clean fixture resources even on failure. These cover AC-UI-TASK-CLEANUP-CONFIRMATION-001.12-.14.

## Work orders

- [x] [Task 01: Enable explicit Quick Chat worktree discard](task-01-quick-chat-consent.md)

## Verification results

Task 01 passed the focused frontend and backend tests, typecheck, localization,
desktop and phone E2E, public-doc validation, and specification validation.
Command details and test conditions are recorded in the work order.

## Risks

- The actual reported failure may be a distinct filesystem error; capture its error if it persists.
- Task confirmation tickets bind options and task scope; stale tickets or late responses must not authorize a changed choice.
- Active agents can change the workspace during inspection; retain backend admission and ownership checks and handle a conflict without losing the chat.
