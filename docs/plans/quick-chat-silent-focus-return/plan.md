---
created: 2026-08-27
status: complete
requirements:
  - REQ-UI-QUICK-TERMINAL-001
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
legacy_specs: []
---

# Implementation Plan: Quick Chat Silent Focus Return

## Overview

Remove the visible focus treatment that appears after Escape closes the shared Quick Chat dialog.
Keep focus on the launcher. Restore its normal keyboard focus treatment after focus leaves once.

One work order owns the focus helper, scoped style, unit tests, and browser regression. This order
keeps the transient state and its executable evidence in one change.

## Scope

### In scope

- Mark automatic launcher focus restoration as visually silent.
- Remove the marker when focus leaves the launcher.
- Keep the launcher focused after the dialog closes.
- Keep the sidebar tooltip closed after focus returns.
- Keep normal focus treatment when keyboard navigation later reaches the launcher.

### Out of scope

- Removing focus indicators from ordinary keyboard navigation.
- Changing pointer dismissal or tooltip hover behavior.
- Changing Quick Chat state, layout, copy, or mobile touch controls.
- Changing Configuration Chat or unrelated dialog focus behavior.

## Technical approach

### Transient focus marker

Update `apps/web/components/quick-chat/quick-chat-focus.ts`. Launcher activations request a
transient data marker before the saved launcher receives focus. Remove the marker on the first blur
event. Global keyboard shortcuts and command-palette actions still restore their origin, but request
no marker. Do not mark or focus an origin that is no longer in the document.

Keep this state in the shared helper. Do not add state to Zustand or to each launcher component.
Quick Chat and Quick Terminal already use this helper through their shared provider.

### Scoped appearance

Add one selector to `apps/web/app/globals.css`. While the transient marker and `:focus-visible` are
both active, remove the outline, ring shadow, and focus border. Scope the selector to the marker.
Do not change the global focus style or the base button component.

### Mobile parity

The nearest mobile exemplar is the current Quick Chat header action in
`apps/web/components/kanban/kanban-header-mobile.tsx`. The existing full-height dialog and explicit
touch close control remain unchanged. This change does not alter layout, navigation, scrolling,
safe areas, or touch targets.

The existing Pixel 5 close scenario remains the rendered mobile check. A new mobile scenario is not
required because the changed state starts only after desktop keyboard dismissal.

## Tests

- `AC-UI-QUICK-TERMINAL-001.9`: Extend
  `apps/web/components/quick-chat/quick-chat-focus.test.ts`. Make sure that restoration adds the
  marker, keeps focus, and removes the marker after blur.
- Cover a non-launcher origin with silent styling disabled, and verify that global Quick Chat and
  Configuration Chat commands disable silent styling.
- Keep the disconnected-launcher regression in the same file. Make sure that no marker remains.
- Keep `apps/web/components/quick-chat/quick-chat-provider-focus.test.tsx` as provider-level evidence
  that a close transition uses the shared restoration helper.

## E2E tests

- `AC-UI-QUICK-TERMINAL-001.9`: Add a focused scenario to
  `apps/web/e2e/tests/chat/quick-chat.spec.ts`. Open Quick Chat from the desktop sidebar. Press
  Escape. Make sure that the launcher is focused with no outline, shadow, or changed focus border.
- In the same scenario, move focus away and return with keyboard navigation. Make sure that the
  normal focus indicator is visible again.
- Cover the global Quick Chat shortcut from an unrelated control. Make sure that focus returns to
  that origin without the silent marker.
- Run the existing `mobile-chrome` home-header close scenario in
  `apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts`. It proves that the touch close path and
  mobile layout remain unchanged.

## Work orders

- [x] [Task 01: Silence Automatic Launcher Focus](task-01-silence-launcher-focus.md)

## Verification results

- Unit tests: 34 passed across the four targeted Vitest files.
- ESLint: passed for all changed frontend source and test files.
- Chromium E2E: 2 passed for launcher and shortcut-origin focus return.
- Pixel 5 E2E: 1 passed for the existing mobile touch-close scenario.
- Specification lint: passed with `python3 scripts/lint-spec-files.py --all`.
- Diff check: passed with `git diff --check`.

## Risks

- A marker that survives blur can hide a later keyboard focus indicator.
- A broad CSS selector can remove focus styles from unrelated controls.
- A browser test that checks only focus ownership can miss the visual regression.
