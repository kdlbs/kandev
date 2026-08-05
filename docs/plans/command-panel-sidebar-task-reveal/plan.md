---
spec: docs/specs/ui/command-panel-sidebar-task-reveal.md
created: 2026-08-05
status: implemented
---

# Implementation Plan: Command-panel Sidebar Task Reveal

## Overview

Add a bounded DOM-navigation helper for rendered task rows, invoke it when Cmd+K opens a task, and
prove the behavior against an actually overflowing sidebar. The helper follows the retryable target
navigation pattern already used by `apps/web/lib/review/navigation.ts`, while scoping lookup to a
visible task-sidebar scroll viewport so the CSS-hidden desktop rail is never selected on mobile.

## Confirmed root cause

`useCommandPanelHandlers.handleTaskSelect` in `apps/web/components/command-panel.tsx` only closes the
palette and pushes `linkToTask(task.id)`. `TaskSidebarScrollArea` in
`apps/web/components/task/task-sidebar-scroll-area.tsx` only tracks its bottom-scroll cue; no code
connects command-panel selection to the nested Radix scroll viewport. Consequently, route and
active-task state change while an off-screen sidebar row remains outside the visible list.

## Frontend

### Sidebar target navigation

- Add `apps/web/lib/sidebar/task-navigation.ts` with a stable task-row DOM attribute/selector and a
  bounded `requestAnimationFrame` retry helper. It resolves the row only inside a rendered, visible
  `task-sidebar-scroll` viewport and calls `scrollIntoView({ block: "nearest", inline: "nearest" })`.
  A missing or hidden target resolves as a no-op rather than affecting navigation.
- Add a row-specific task marker to the interactive row in
  `apps/web/components/task/task-item.tsx`. Keep the existing test ID and active-task accessibility
  attributes intact.
- Update `useCommandPanelHandlers.handleTaskSelect` in
  `apps/web/components/command-panel.tsx` to start the sidebar reveal alongside the existing
  canonical task navigation. Do not move focus or change active sidebar view/collapse state.

### Mobile design contract

Desktop outcome: Cmd+K opens the selected task and the rendered active row becomes visible inside
the existing task-list scroll owner. Phone entry point and presentation remain the existing direct
task route plus the optional `SessionTaskSwitcherSheet` inset drawer; the command-panel action does
not open that drawer. `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` is the
nearest shipped exemplar and contributes the rule that phone task navigation owns a separate,
safe-area-aware internal scroller. Shared task search and route logic remain unchanged; the reveal
helper selects only a layout-visible desktop scroll viewport. No new touch target, viewport-bound
surface, or safe-area behavior is introduced.

## Tests

- **What:** selector escaping, immediate and delayed row discovery, minimum-distance scroll options,
  visible-sidebar scoping, and bounded failure.
- **File:** `apps/web/lib/sidebar/task-navigation.test.ts`.
- **How:** Vitest/jsdom with explicit layout visibility and queued animation-frame callbacks.
- **Regression order:** first add the overflowing-sidebar browser scenario and run it against the
  current code to confirm the selected route changes while the target row stays outside the sidebar
  viewport; then implement the helper and rerun to green.

## E2E tests

- **Scenario:** Given enough desktop tasks to overflow the sidebar, when Cmd+K selects a task whose
  row is confirmed outside the nested viewport, then the URL, active-row state, and row/container
  bounding boxes prove the row was revealed.
- **File:** `apps/web/e2e/tests/task/sidebar-scroll-preservation.spec.ts`.
- **What to verify:** precondition overflow and off-screen target, command-panel selection through
  the UI, canonical `/t/:id` navigation, `aria-current`, containment within
  `task-sidebar-scroll`, and no document-scroll movement.
- **Scenario:** Given a phone viewport, when Cmd+K selects a task, then direct task navigation still
  succeeds without revealing the hidden desktop rail.
- **File:** `apps/web/e2e/tests/task/mobile-command-panel-task-navigation.spec.ts`.
- **What to verify:** canonical task URL and usable mobile task surface; the desktop sidebar remains
  non-visible and document horizontal overflow is absent.

## Verification results

- Dependency setup: `cd apps && pnpm install --frozen-lockfile` passed.
- RED regression: the focused desktop E2E failed before the production wiring with the route
  changing while the selected row remained outside the sidebar viewport.
- Unit: `cd apps && pnpm --filter @kandev/web test -- --run lib/sidebar/task-navigation.test.ts` —
  1 file, 6 tests passed.
- Typecheck: `cd apps/web && pnpm run typecheck` passed.
- Desktop GREEN: focused `sidebar-scroll-preservation.spec.ts` — 1 test passed.
- Mobile GREEN: focused `mobile-command-panel-task-navigation.spec.ts` under `mobile-chrome` — 1
  test passed.
- Formatting and lint: focused Prettier check and ESLint with `--max-warnings 0` passed.
- Repository checks: `git diff --check` passed. Managed E2E build/test artifacts were disposable
  and are not part of the change.

## Implementation waves

- [x] [Task 01 — reveal command-selected sidebar task](task-01-reveal-command-selected-task.md) — done

Execution is sequential in the primary conversation. No subagents are authorized.

## Risks and boundaries

- The active sidebar view may intentionally omit or collapse the task. The helper must time out
  without mutating persisted user preferences.
- Desktop and mobile sidebar DOM can coexist because the desktop rail is hidden with responsive
  CSS. Visibility scoping is required to avoid treating hidden markup as the destination.
- `content-visibility: auto` is used for long task lists. The E2E assertion must inspect the row's
  actual bounding box after reveal rather than assuming DOM presence proves visibility.
