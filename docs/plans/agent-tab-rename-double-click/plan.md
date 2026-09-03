---
created: 2026-08-31
status: done
requirements:
  - REQ-UI-TASK-AGENT-TAB-RECONCILIATION-002
system_design:
  - ../../specs/ui/system-design/task-agent-tab-reconciliation.md
legacy_specs: []
---

# Implementation Plan: Isolate Agent Tab Rename Double-clicks

## Overview

Keep text-selection double-clicks inside the shared tab rename editor so they
cannot reach the surrounding Agent-tab maximize handler. One work order adds a
browser regression first, applies the shared event-boundary correction, and
runs the focused frontend checks.

## Scope

### In scope

- Preserve native double-click text selection in an active Agent-tab rename
  input.
- Prevent that double-click from maximizing or restoring the Dockview group.
- Preserve tab-level double-click maximize behavior outside rename mode.
- Apply the isolation through the shared task tab rename editor.

### Out of scope

- Session-name persistence, validation, commit, cancel, or error behavior.
- Dockview maximize state management or layout persistence.
- Phone and tablet session-control composition.
- Quick Chat tab rename behavior, which uses a different editor.

## Confirmed root cause

`SessionTab` mounts `TabRenameInput` inside a `ContextMenuTrigger` whose
`onDoubleClick` calls `useTabMaximizeOnDoubleClick`. The editor stops bubbling
`mousedown` and `click`, but browsers emit a separate bubbling `dblclick` event.
That event reaches the parent maximize handler, which prevents the default text
selection and toggles the Dockview group.

## Technical approach

- Add a focused Chromium Playwright scenario in
  `apps/web/e2e/tests/session/session-tab-rename.spec.ts`. Enter Agent-tab rename
  mode from the context menu, double-click the input, assert native selection,
  and assert that the group remains unmaximized.
- Update `TabRenameInput` in
  `apps/web/components/task/tab-rename-input.tsx` to stop propagation of the
  shared editor's `dblclick` event without calling `preventDefault`.
- Leave `SessionTab` and `useTabMaximizeOnDoubleClick` unchanged so ordinary
  tab-surface double-clicks retain their current maximize-or-restore behavior.

## Tests

- `AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.2`: the Playwright regression
  verifies selected input text and unchanged maximize state after an editor
  double-click.
- `AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.3`: the same scenario verifies the
  existing tab-surface maximize path remains available outside the editor.

## E2E tests

- Chromium:
  `apps/web/e2e/tests/session/session-tab-rename.spec.ts` covers the affected
  fine-pointer Dockview workflow.
- No mobile E2E change is required because phone and tablet task surfaces do
  not mount desktop Dockview Agent tabs or their maximize gesture.

## Work orders

- [x] [Task 01: Isolate Rename Input Double-clicks](task-01-isolate-rename-input-double-clicks.md)

## Verification results

Completed on 2026-08-31.

- The Chromium regression failed before the fix because the rename-input
  double-click removed the group's maximize icon by maximizing the group.
- The rebuilt production E2E bundle passed the focused scenario after the
  shared editor stopped the bubbling `dblclick` event.
- Web typecheck, focused ESLint, and `git diff --check` passed.
- No mobile test changed because phone and tablet do not mount the affected
  Dockview Agent-tab surface.

## Risks

- Calling `preventDefault` at the editor boundary would fix maximize while also
  breaking the text-selection behavior being protected. The correction must
  stop propagation only.
- Testing only the two individual click events would miss the separately
  dispatched `dblclick` that causes the regression.
