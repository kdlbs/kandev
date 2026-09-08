---
spec: docs/specs/ui/requirements/plan-editor-task-switch-stability.md
created: 2026-09-08
status: implemented
---

# Implementation Plan: Plan Editor Task-Switch Stability

## Overview

Prevent the Plan panel from sending search commands to an editor that belongs
to the outgoing task or has already been destroyed.

## Confirmed root cause

The deployed exception is `TypeError: Cannot read properties of null (reading
'commands')`. It starts in TipTap's `Editor.commands` getter and maps to
`usePlanFindShortcut`.

`TaskPlanPanel` stores the editor in parent state. The editor child uses a task
ID in its React key, so React destroys that child during a task switch. The
parent state can still expose the old editor for one render.

TipTap 3.31 clears the command manager during editor destruction. The plan
search hook then calls `clearPlanSearch()` on the destroyed editor. This latent
lifetime error became fatal after the dependency update from TipTap 3.19.

## Frontend

### Task-scoped editor ownership

- Store the editor together with its owner task ID.
- Return no editor when the stored owner does not match the selected task.
- Bind the editor-ready callback to its current task ID.
- Apply the same ownership and destroyed-editor checks to plan-comment commands.

### Destroyed-editor command gate

- Add one availability check for null or destroyed editors.
- Use the check before state reads, event subscriptions, cleanup, and commands.
- Keep search actions as no-ops until the replacement editor is ready.
- Preserve current query, match count, shortcut, and navigation behavior.

### Mobile design contract

Desktop and phone layouts use the same hook and editor ownership. The fix does
not change mobile composition, gestures, scrolling, or touch targets. Focused
unit tests and the desktop task-switch regression cover the changed behavior.

## Tests

- **What:** A destroyed, non-null editor is unavailable to plan search.
- **File:** `apps/web/components/task/use-plan-find-shortcut.test.tsx`.
- **How:** Use an editor double whose `commands` getter throws and assert that
  the hook does not access editor methods.
- **What:** An editor from the outgoing task is unavailable during replacement.
- **File:** `apps/web/components/task/task-plan-panel.session-switch.test.tsx`.
- **How:** Change the task ID before the new editor-ready callback and verify
  task ownership and plan-comment command safety at each render.

The focused unit or component regression must fail before production changes.
It must pass after the change.

## E2E test

- **Scenario:** Keep the Plan panel open while selecting a second sidebar task.
- **File:** `apps/web/e2e/tests/search/plan-search.spec.ts`.
- **What to verify:** The route recovery screen does not appear, no page error
  occurs, and plan search works in the selected task.

## Implementation waves

- [x] [Task 01: Guard the plan editor lifecycle](task-01-guard-plan-editor-lifecycle.md)

Execution is sequential in the primary conversation. No subagents are
authorized.

## Risks and boundaries

- React can run an effect cleanup after TipTap destroys its editor. Cleanup
  must not assume that editor methods are available.
- A late editor-ready callback must not restore an editor from the old task.
- The fix does not change task selection, plan APIs, layout persistence, route
  recovery, voice services, analytics, or TipTap dependency versions.
- The adjacent duplicate-extension warning is not the cause of this crash and
  is outside this plan.
