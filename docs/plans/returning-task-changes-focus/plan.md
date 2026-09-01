---
spec: docs/specs/ui/requirements/task-layout-profiles.md
created: 2026-07-31
status: complete
---

# Implementation Plan: Restore Returning Task Changes Focus

## Overview

Restore the desktop workbench attention behavior for Git or commit updates that arrive while a task is inactive. The confirmed regression is the `hasSavedLayout` branch in `applyChangesPanelAutoFocusState`, introduced by `ba8d99f33`, which deletes pending focus instead of activating Changes for every previously opened task. The repair removes that blanket suppression while retaining first-observation baselining, restore-race deferral, page-refresh focus preservation, and the Agent-group safety check.

## Frontend

### Changes attention state

- `apps/web/components/task/changes-panel-focus.ts`: remove the saved-layout lookup and `hasSavedLayout` branch from `applyChangesPanelAutoFocusState`. A pending environment remains deferred while Dockview is restoring and activates Changes after restoration even when an environment layout exists.
- Preserve `markInactiveChangesIncreases` semantics: an already-known non-zero count alone does not create attention on reload; only a count increase or meaningful non-zero fingerprint change while inactive queues attention.
- Preserve `activateChangesPanel` semantics: a missing Changes panel is a terminal no-op, while a Changes panel grouped with Agent sessions is not force-activated.

### Responsive boundary

- `TaskLayout` mounts `DockviewDesktopLayout` only for the desktop workbench; mobile and tablet use their existing dedicated layouts. This repair changes no mobile composition, touch target, scrolling, navigation, or saved mobile state.
- No mobile Playwright case is added because the affected hook is not mounted on mobile or tablet. Existing mobile task navigation remains the applicable parity path.

## Tests

- **Returning saved layout with pending changes:** update `apps/web/components/task/changes-panel-focus.test.ts` so a pending environment activates Changes despite having a saved layout. Write and run this expectation before changing production code; it must fail because the current branch clears the pending key.
- **Reload with existing changes:** retain the first-observation and active-environment unit coverage proving an already-known non-zero marker does not queue attention.
- **Restore race and panel safety:** retain coverage for restore deferral, missing panels, and Agent-group blocking.

## E2E Tests

- **Scenario:** GIVEN a desktop task with a saved Agent/Files panel and a Changes panel outside the Agent group, WHEN a Git update is detected while the task is inactive and the user returns, THEN Changes is active after layout restoration.
- **File:** `apps/web/e2e/tests/layout/changes-panel-focus.spec.ts`.
- Rename and reverse the regression added by `ba8d99f33`; run it against the current implementation before the production change and confirm it fails because the saved panel remains active.
- Keep the existing page-refresh scenario to prove already-known changes do not steal focus.

## Implementation Tasks

- [x] [Task 01 - Restore pending Changes attention](task-01-restore-pending-changes-attention.md)

Execution is sequential in the primary conversation. There are no parallel-safe candidates because the unit test, state helper, and E2E scenario encode one behavior and must follow one Red-Green-Refactor cycle.

## Risks

- Removing too much baselining could make async Git hydration steal focus on page reload. The repair must remove only the saved-layout suppression.
- Activating before Dockview restoration completes can target a stale or missing panel. Existing restore deferral and retry behavior must remain intact.
- Changes must not steal focus when it shares a group with Agent session panels.
- The worktree currently lacks `apps/node_modules`; run `pnpm install --frozen-lockfile` from `apps/` before the RED checks.

## Out of Scope

- Focusing Changes for every task with an already-known dirty workspace.
- Changing mobile or tablet task-detail layouts.
- Changing Dockview layout persistence, panel placement, or Git-status transport.
- Public documentation changes; this restores previously intended workbench behavior without changing a public API or documented workflow.

## Verification Results

- RED unit: the saved-layout regression failed with `expected 0 to be 1`; the other 14 tests passed.
- RED E2E: Changes remained `dv-inactive-tab` after returning to the saved task.
- GREEN unit: `components/task/changes-panel-focus.test.ts` — 16 tests passed after the
  maximized-layout review regression was added.
- Typecheck: `pnpm run typecheck` from `apps/web` — passed.
- GREEN E2E: focused Chromium production-build scenario — 1 test passed in 1.9s.
- Desktop PR capture:
  `rtk pnpm e2e:run tests/layout/pr-capture.spec.ts -- --project=chromium` — temporary
  synthetic capture scenario passed and produced the 1280×720 desktop screenshot.
- Mobile PR capture:
  `rtk pnpm e2e:run tests/layout/mobile-pr-capture.spec.ts -- --project=mobile-chrome` —
  temporary synthetic Pixel 5 capture scenario passed. The capture specs were removed after
  producing the ignored PR assets.
- PNG compression:
  `rtk pnpm dlx pngquant-bin@9.0.0 --quality 65-90 --ext .png --force web/.pr-assets/*.png`
  — passed before both screenshots were embedded in PR #2091.
- Whitespace: `git diff --check` — passed.
- Mobile: no product E2E regression was added because `useChangesPanelAutoFocus` is mounted
  only by `DockviewDesktopLayout`; mobile and tablet use separate task layouts and received no
  state or presentation changes.
