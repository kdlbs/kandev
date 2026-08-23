---
spec: docs/specs/ui/session-tab-delete-feedback.md
created: 2026-08-05
amended: 2026-08-22
status: shipped
---

# Implementation Plan: Session tab delete feedback

## Overview

Give the desktop Dockview session-tab close flow local pending feedback while preserving the shared
session lifecycle behavior used by context menus and mobile. First make the shared delete action
selectively suppress progress/success toasts, then replace Dockview's opaque close icon with a
small repository-owned action that can render an X or spinner. Finish with focused desktop and
mobile Playwright coverage. A later localized-confirmation refinement keeps the tab-X dialog,
anchors context-menu confirmation to its Delete item, and morphs the phone picker row into inline
touch actions.

## Frontend

### Shared session delete action

Update `apps/web/hooks/domains/session/use-session-actions.ts` so `remove` accepts an optional
feedback mode and returns whether deletion succeeded. Keep the current loading-to-success toast
sequence as the default for existing callers. Add an error-only mode for the tab-X flow: it emits
no loading or success toast, but emits one error toast on rejection and leaves store/panel cleanup
behind the success result. Existing active-session handoff and `onDeleted` ordering remain intact.

### Session tab close action

Add `apps/web/components/task/session-tab-close-action.tsx` as the repository-owned close action.
It preserves Dockview's `dv-default-tab-action` class and the existing
`session-tab-close-<sessionId>` test ID, prevents the close target from becoming tab-activation
intent, and renders either `IconX` or `GridSpinner`. While pending it is disabled, carries
`aria-busy`, and retains a localized accessible name.

Update `apps/web/components/task/session-tab.tsx` to render `DockviewDefaultTab` without its opaque
close control and place the new action in the same tab chrome. Track whether the open confirmation
came from the X or the context menu. Only confirmed X-originated deletion uses error-only feedback
and drives the close spinner; context-menu deletion retains the default toast mode. Cancelling the
dialog clears the origin without entering the pending state.

Add the close action's accessible label to `apps/web/src/locales/en/common.json` and
`apps/web/src/locales/zh-cn/common.json`, then regenerate
`apps/web/src/locales/pseudo/common.json` with `pnpm run i18n:pseudo`.

### Mobile design contract

- **Desktop outcome:** the session-tab X remains the progress surface and retains its alert dialog.
  Context-menu Delete keeps the menu mounted and opens a compact confirmation popover anchored to
  that item.
- **Mobile entry point:** the existing `MobileSessionsPicker` pill opens `MobilePickerSheet`, and a
  session row's visible actions menu owns deletion. The target row then morphs into touch-sized
  inline Cancel and Delete actions; there is no Dockview X or nested confirmation dialog on phone.
- **Nearest shipped exemplar:** `apps/web/components/task/mobile/mobile-sessions-section.tsx`
  remains authoritative for the phone hierarchy, bottom-sheet surface, internal scroll owner,
  safe-area handling, and touch targets.
- **Shared versus specialized behavior:** the delete transport/store mutation and localized warning
  copy remain shared. Desktop context-menu and phone picker components specialize only the local
  confirmation presentation; their default request feedback remains unchanged.
- **Dismissal:** Cancel, session selection, and externally closing the picker clear pending inline
  confirmation without dispatching deletion.
- **Parity proof:** desktop Playwright scenarios cover the anchored context-menu confirmation. A
  mobile scenario cancels and then confirms the inline row action, proving the selected session
  disappears while the remaining session stays reachable.

## Tests

- **Feedback modes and cleanup:** extend
  `apps/web/hooks/domains/session/use-session-actions.test.ts` to prove default callers still receive
  loading/success toasts, error-only deletion receives neither, failures receive one error toast,
  `remove` reports success/failure, and store/panel cleanup remains success-only.
- **Close action state:** add
  `apps/web/components/task/session-tab-close-action.test.tsx` to prove the idle X is operable, the
  pending state renders a status spinner with `aria-busy`, and pending activation cannot dispatch a
  second callback.
- **Localized confirmation surfaces:** `session-tab-menu.test.tsx` proves the context menu remains
  mounted for its anchored popover. `mobile-sessions-section.test.tsx` proves inline confirm,
  cancellation, dispatch, touch-target sizing, and cleanup when the picker closes externally.

## E2E Tests

- **Scenario:** GIVEN two deletable sessions, WHEN the desktop user clicks a tab X and confirms,
  THEN the tab/session is removed and no `Deleting session...` or successful-deletion toast appears.
  **File:** `apps/web/e2e/tests/session/session-tab-management.spec.ts`.
- **Scenario:** GIVEN two deletable sessions on a phone viewport, WHEN the user opens the Sessions
  picker and chooses Delete, THEN the row shows inline confirmation with no alert dialog; cancelling
  preserves the row, and confirming removes it while the remaining session stays reachable.
  **File:** `apps/web/e2e/tests/session/mobile-session-deletion.spec.ts`, run by the `mobile-chrome`
  project.
- **Scenario:** GIVEN a desktop session context menu, WHEN the user chooses Delete, THEN a compact
  anchored popover appears without an alert dialog and the existing cancel/confirm deletion outcomes
  remain intact. **Files:** `apps/web/e2e/tests/session/multi-session-ux.spec.ts` and
  `apps/web/e2e/tests/session/session-tab-management.spec.ts`.

The transient spinner is covered deterministically by the component test because the real delete
response can settle too quickly for a race-free Playwright observation. Playwright covers the
integrated no-toast result and session reconciliation.

## Verification Results

- `rtk pnpm --filter @kandev/web test -- components/task/session-tab-close-action.test.tsx hooks/domains/session/use-session-actions.test.ts` — 2 files, 15 tests passed.
- `rtk pnpm run typecheck` — passed.
- `rtk pnpm run i18n:check` — passed; pseudo locale is synchronized. The existing catalog audit
  remains advisory with 670 zh-cn parity notices.
- `rtk pnpm run i18n:ratchet` — passed.
- `rtk pnpm exec eslint components/task/session-tab.tsx components/task/session-tab-close-action.tsx hooks/domains/session/use-session-actions.ts hooks/domains/session/use-session-actions.test.ts components/task/session-tab-close-action.test.tsx e2e/tests/session/session-tab-management.spec.ts e2e/tests/session/mobile-session-deletion.spec.ts` — passed with no errors or warnings.
- `rtk pnpm exec prettier --check ...` — all changed source, test, locale, spec, and plan files
  passed formatting checks.
- `rtk git diff --check` — passed.
- `rtk pnpm e2e:run tests/session/session-tab-management.spec.ts -- --grep "tab close button shows delete confirmation"` — 1 desktop test passed with a production build.
- `rtk pnpm e2e:run --no-build --project mobile-chrome tests/session/mobile-session-deletion.spec.ts` — 1 mobile test passed.
- `rtk pnpm e2e:run --no-build tests/session/session-tab-management.spec.ts tests/session/session-tab-close-guard.spec.ts` — 9 desktop tests passed.

The pseudo-locale artifact updated is `apps/web/src/locales/pseudo/common.json`. Managed E2E runs
used isolated temporary backends and cleaned their generated results; no external systems were
changed.

### Review remediation (2026-08-22)

- `pnpm --filter @kandev/web test -- components/task/mobile/mobile-sessions-section.test.tsx components/task/session-tab-menu.test.tsx` - 2 files, 17 tests passed.
- `pnpm run typecheck` - passed.
- Targeted ESLint for the shared description, desktop menu, mobile picker, and regression test -
  passed with no warnings.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` - passed; catalogs remain complete and four
  modified frontend files passed the new-code ratchet.
- Local Playwright was not repeated because remediation changes only test coverage, comments,
  component ownership, and internal documentation; exact-head CI reruns the integrated desktop and
  mobile scenarios.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-inline-delete-feedback](task-01-inline-delete-feedback.md)

Wave 2:

- [x] [task-02-session-deletion-e2e](task-02-session-deletion-e2e.md)

Execution is sequential in the primary conversation. The E2E task depends on the production and
unit-test changes from task 01; these tasks are not parallel-safe.

## Public documentation

No public documentation change. This refines transient confirmation presentation without changing
commands, configuration, navigation terminology, or a public API.

## Risks

- Dockview owns its default X markup, so the custom action must preserve the
  `dv-default-tab-action` class, pointer suppression, visibility behavior, and activation-intent
  guard while avoiding a fork of the full tab implementation.
- X-origin tracking must be reset on cancel and failure so a later context-menu delete cannot
  inherit error-only feedback or a stale spinner.
- The backend delete response can be faster than a rendered frame; deterministic transient-state
  proof belongs in the component test rather than a timing-sensitive E2E assertion.
