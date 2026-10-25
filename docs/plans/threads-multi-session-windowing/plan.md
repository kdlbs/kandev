---
created: 2026-08-28
status: completed
requirements:
  - ../../specs/ui/requirements/threads-conversation-deck.md
  - ../../specs/platform/requirements/viewport-bounded-session-delivery.md
system_design:
  - ../../specs/ui/system-design/threads-conversation-deck.md
  - ../../specs/platform/system-design/viewport-bounded-session-delivery.md
legacy_specs: []
---

# Implementation Plan: Threads Multi-session Windowing

## Overview

Extend the contributor Threads view so each task column can switch among its
existing agent sessions without adding management actions or another header
row. Correct the question icon so only an explicit clarification or permission
means that a person must act. Bound expensive conversation work by the
horizontal viewport: task shells stay stable, nearby session lists preload,
and only the selected session in a visible column owns a transcript and full
session subscription.

This package targets PR #3112. It preserves the contributor's first-class
Threads route, stable task order, shared composer path, task-page round trip,
and mobile snap-paged deck.

## Scope

### In scope

- A compact, workspace-scoped per-session pending-action event with revision
  ordering and no transcript content.
- Stable task shells with separate preload and detail activation windows.
- One selected transcript subscription per visible desktop task column and one
  total selected transcript on phone.
- Column-local session selection, exact task/session deep links, and
  deterministic fallback when membership changes.
- Desktop switch-only tabs on the right of the existing metadata row.
- A phone pill and bottom-sheet session picker with 44-pixel rows.
- Correct clarification, permission, working, turn-finished, and review-ready
  status semantics.
- Unit, component, desktop E2E, mobile E2E, and WebSocket traffic evidence.

### Out of scope

- Session creation, deletion, rename, reorder, pin, primary-session mutation,
  or persistence of Threads selection.
- Full task-shell DOM virtualization.
- Task status summary session arrays or transcript projections.
- Changes to the task workbench's global active-session behavior.
- Plugin API changes.

## Technical approach

### Compact sibling status

Keep `session.message.*` events session-scoped. Add
`session.pending_action_changed` as a workspace-scoped semantic event derived
from the current authoritative pending-action projection. Reuse
`pending_action_revision` for cross-channel stale-event rejection. Route the
event fail-closed by workspace and merge it into the existing compact
`TaskSession` store fields. Existing global `session.state_changed` continues
to supply lifecycle and foreground activity.

### Viewport activation

Keep all `ActiveThread` task shells mounted so stable order, flexible column
width, horizontal scroll, deep-link focus, and accessibility order do not
change. Add an `IntersectionObserver` rooted at `ThreadsBoard`.

- The preload set contains intersecting desktop columns plus one adjacent
  column on each side. It mounts `useTaskSessions` only.
- The desktop detail set contains intersecting columns. It mounts one
  `TaskChatPanel` for each column's selected session.
- The phone detail set contains only the nearest snapped column. The next
  column can preload membership but cannot subscribe to messages.
- Leaving the detail set unmounts the selected chat and releases its session
  registration. The task shell and local session selection remain.

A component test with 30 task shells is the main resource-bound oracle. It
asserts that initial session-list and chat mounts follow the two viewport sets,
not the task count.

### Session selection and presentation

Load session membership through `useTaskSessions` only after preload
activation. Build a pure session view model from existing session sort/profile
helpers and compact pending status. Keep selected IDs in Threads-local state
keyed by task ID.

Desktop renders switch-only `SessionTabs` in the right side of the existing
metadata row. The right region scrolls internally if names do not fit. Do not
pass add, close, context-menu, double-click, drag, or management callbacks.

Phone renders a `MobilePillButton` in the same metadata row and a
`MobilePickerSheet` for the session list. This avoids a nested horizontal tab
gesture inside the horizontally paged task deck.

`ThreadConversation` continues to omit `onSend`, so `TaskChatPanel` owns queue
mode, model, plan mode, context, attachments, optimistic insertion, and send
reconciliation.

### Status semantics

Use a shared pure helper with explicit precedence: permission, clarification,
starting, working, terminal error, then neutral settled state. Never infer a
question from `WAITING_FOR_INPUT`. Use task review outcome for the
`Ready for review` completion treatment. Task-wide pending action can rank or
decorate a task shell, but only compact per-session projection can decorate a
specific tab.

Add every new label to all five locale catalogs. Generate Traditional Chinese
translations with the existing `i18n:zh-hant` workflow when implementation
adds the keys.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-002.1` through `.6` | Backend event routing/payload tests and frontend revision-aware reducer tests. |
| `AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.1` through `.6` | Controllable IntersectionObserver component tests with 30 shells and mocked conversation/list mounts. |
| `AC-UI-THREADS-DECK-001.1` through `.7` | Pure selection tests, component switcher tests, and desktop task/session deep-link E2E. |
| `AC-UI-THREADS-DECK-002.1` through `.6` | Status precedence unit matrix plus inactive-tab live-update E2E. |
| `AC-UI-THREADS-DECK-003.1` through `.3` | Thirty-shell component budget and desktop scroll/subscription E2E. |
| `AC-UI-THREADS-DECK-003.4` through `.7` | Mobile picker component tests and mobile-chrome geometry, touch, failure, and scroll assertions. |

## E2E tests

- Extend `apps/web/e2e/tests/task/threads-view.spec.ts` with one real primary
  session plus existing test-harness sibling sessions. Verify same-row tabs,
  switch-only controls, selected transcript changes, inactive-tab status,
  exact `taskId` plus `sessionId` round trip, and stable task-column order.
- Add a multi-column traffic scenario that records outgoing WebSocket actions.
  Verify initial `session.subscribe` IDs equal selected sessions in visible
  columns, unselected sibling IDs never subscribe, scrolling activates the new
  selected session, and the departed offscreen session unsubscribes.
- Extend `apps/web/e2e/tests/task/mobile-threads-view.spec.ts` with a
  multi-session task. Verify one snapped detail owner, the metadata-row pill,
  bottom-sheet selection, 44-pixel rows, status parity, safe-area containment,
  and zero document overflow.
- Keep the exact 30-task resource bound in deterministic component coverage.
  The E2E fixture does not need to run 30 real agent turns to prove the same
  mount and subscription invariant.

## Mobile design contract

- **Desktop outcome:** compact existing-session tabs share the metadata row;
  visible columns can each show one selected conversation.
- **Mobile entry and hierarchy:** the task deck remains horizontally
  snap-paged. A compact session pill in the metadata row opens a bottom sheet.
- **Surface rationale:** a bottom sheet avoids nested horizontal gestures and
  gives long labels and status text a full-width 44-pixel row.
- **Scroll ownership:** the board owns horizontal task paging; the picker owns
  vertical session scrolling; the transcript owns its internal vertical
  scroll. The page document gains no horizontal overflow.
- **Shared behavior:** membership, selection fallback, session status, and
  conversation submission are shared. Only the selector presentation and
  phone detail-window rule differ.
- **Mobile proof:** mobile-chrome validates one active detail, picker geometry,
  touch targets, status parity, safe area, and zero document overflow.

## Work orders

- [x] [Task 01: Publish compact session attention](task-01-publish-compact-session-attention.md)
- [x] [Task 02: Activate task columns from the viewport](task-02-activate-task-columns-from-viewport.md)
- [x] [Task 03: Add the Threads session switcher](task-03-add-threads-session-switcher.md)
- [x] [Task 04: Prove the Threads resource budget](task-04-prove-threads-resource-budget.md)

All work orders are sequential. They share event types, task-session state,
Threads components, and E2E fixtures, so this plan has no parallel-safe
implementation candidates.

## Risks

- A pending-action event must not broaden delivery of message content. Assert
  the exact payload and workspace routing.
- React effect cleanup during fast horizontal scroll can briefly overlap old
  and new chat mounts. Assertions must permit transition overlap only while
  guaranteeing the settled subscription set.
- A partially visible phone neighbor must not become a second detail owner.
  Use the nearest snap target instead of raw intersection on phone.
- A task-wide pending action has no session identity. Never assign it to the
  primary tab before the task-session list identifies the owner.
- Many compact tabs can compete with metadata. Keep one row, bound the right
  region, and test long labels at compact desktop widths.
