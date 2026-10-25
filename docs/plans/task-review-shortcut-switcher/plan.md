---
spec: docs/specs/ui/requirements/task-review-shortcut.md
created: 2026-07-31
status: completed
---

# Implementation Plan: Task Review Shortcut Switcher

## Overview

Turn the existing multi-review shortcut picker into a controlled, held-chord
switcher. Reuse the recent-task switcher's tested shortcut-key helpers for
platform-aware hold-modifier detection, deliberate key presses, and release
commit; keep provider lookup, ordering, and external-link behavior unchanged.

## Backend

No backend or protocol changes. Existing task PR and MR collections already
contain every target and provider URL needed by the switcher.

## Frontend

### Provider-neutral review targets

- Update `apps/web/components/task/task-pr-open.ts` to expose an ordered,
  discriminated review-target shape built from GitHub PRs followed by GitLab
  MRs. Keep the existing `none` / single direct-open / multiple pick decision,
  but let the controller and picker share one target list and URL source.
- Extend `apps/web/components/task/task-pr-open.test.ts` to prove mixed-provider
  order, direct-open resolution, and target identity.

### Held shortcut controller

- Add `apps/web/components/task/use-task-review-shortcut.ts` and its focused
  test. The hook owns open state, selected target index, synchronous refs for
  key-event ordering, and cancel/commit lifecycle.
- Reuse `hasHoldModifier`, `isCycleShortcutEvent`, and
  `isCommitReleaseEvent` from
  `apps/web/components/task/recent-task-switcher-keys.ts`; do not create another
  modifier parser.
- On a multi-target keydown, open on index 0 or advance one row with wrap.
  After the initial chord, keeping the primary hold modifier alone is enough to
  continue cycling; ignore `KeyboardEvent.repeat`. Commit only on release of
  the configured primary hold modifier. A modifierless binding stays open for
  Enter/click.
- Cancel on Escape, window blur, document hiding, or dialog dismissal so a
  later keyup cannot open a review. Use latest target/index refs so release
  never opens a stale row.
- Update `apps/web/components/task/task-pr-shortcut.tsx` to supply the current
  configurable binding, toast/direct-open callbacks, and controlled picker
  props while preserving capture-phase shortcut precedence.

### Controlled picker selection

- Update `apps/web/components/task/task-pr-picker-dialog.tsx` to render the
  provider-neutral target list and accept controlled selected-index and
  activation callbacks.
- Keep focused-row behavior synchronized with selection. Preserve ArrowUp,
  ArrowDown, Enter, click, Escape, wrapping, scroll containment, and accessible
  focus. Expose stable selected state for E2E without changing row order or
  visual composition.
- Add `apps/web/components/task/task-pr-picker-dialog.test.tsx` for controlled
  focus, arrow wrapping, Enter/click activation, and mixed PR/MR row order.

## Mobile design contract

- Desktop outcome: hold and repeat the configured shortcut, then release its
  primary modifier to open the selected linked review.
- Mobile/coarse-pointer entry points remain the shipped PR/CI drawer and Review
  selector patterns covered by
  `apps/web/e2e/tests/pr/mobile-pr-ci-chip.spec.ts` and
  `apps/web/e2e/tests/review/mobile-review-multi-pr.spec.ts`.
- Picker hierarchy, dialog surface, scroll owner, touch targets, viewport
  sizing, and safe-area behavior do not change. Review data and open actions
  stay shared; only hardware-keyboard event handling changes.
- No new mobile composition or mobile-only test is required because rendered
  layout and touch behavior are unchanged. Existing mobile specs remain the
  parity evidence and run with the focused E2E task.

## Tests

- **What:** provider-neutral PR/MR target ordering and direct/pick resolution.
  **File:** `apps/web/components/task/task-pr-open.test.ts`.
  **How:** table-driven Vitest cases.
- **What:** first press, deliberate repeat, wrap, primary-modifier release,
  Shift-only release, Escape/blur/visibility cancellation, custom modified
  binding, modifierless binding, OS-repeat suppression, and current-target
  commit.
  **File:** `apps/web/components/task/use-task-review-shortcut.test.ts`.
  **How:** `renderHook` with synthetic keydown/keyup and mocked open/toast
  callbacks.
- **What:** controlled focus and legacy Arrow/Enter/click interaction across
  mixed PR/MR rows.
  **File:** `apps/web/components/task/task-pr-picker-dialog.test.tsx`.
  **How:** Testing Library component interaction.

## E2E tests

- **Scenario:** first default chord opens on the first row; deliberate repeated
  G presses advance through GitHub PR and GitLab MR rows and wrap; releasing
  Shift alone keeps the picker open; releasing Command/Control opens the
  selected provider URL.
  **File:** `apps/web/e2e/tests/pr/pr-open-shortcut.spec.ts`.
  **What to verify:** `data-selected` movement, picker visibility, no popup on
  Shift release, final popup URL, and picker close.
- **Scenario:** Escape cancels a held switcher and later primary-modifier
  release opens no provider page.
  **File:** `apps/web/e2e/tests/pr/pr-open-shortcut.spec.ts`.
  **What to verify:** picker closes, page count stays unchanged, and task route
  remains active.
- **Scenario:** one linked review still opens directly and the shortcut remains
  visible in keyboard settings.
  **File:** `apps/web/e2e/tests/pr/pr-open-shortcut.spec.ts`.
  **What to verify:** retain the existing direct-open and settings assertions.

## Implementation waves

Wave 1:

- [x] [Task 01 — held review switcher](task-01-held-review-switcher.md)

Wave 2:

- [x] [Task 02 — shortcut switcher E2E](task-02-shortcut-switcher-e2e.md) —
  depends on Task 01

Execute sequentially in the primary conversation. Waves do not authorize
subagents.

## Risks

- React state can lag a rapid keydown/keyup sequence; synchronous refs must own
  the event-time selected index and open state.
- Releasing Shift must not commit the default `Cmd/Ctrl+Shift+G` binding; only
  the helper-selected primary hold modifier commits.
- Provider lists can update while the picker is open; selection must clamp to
  a current target and never open a removed URL.
- Browser and Tauri external-link paths differ, so unit tests mock the opener
  while Playwright verifies the browser popup URL.
