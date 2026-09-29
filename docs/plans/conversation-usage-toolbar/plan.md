---
created: 2026-09-29
status: complete
requirements:
  - REQ-COSTS-CONVERSATION-USAGE-003
system_design:
  - ../../specs/costs/system-design/conversation-usage.md
legacy_specs: []
---

# Implementation plan: Chat Usage control placement

## Overview

Move the existing conversation Usage disclosure from the transcript footer into the status row above the composer. Show only its stats icon in that row. Preserve its usage data, visibility conditions, desktop popover, and phone drawer.

The costs system owns this placement because the control is the entry point for its built-in conversation usage display. Desktop Approve/Deny buttons already retain their pre-PR compact styling; this package does not change permission actions.

## Scope

### In scope

- Mount one Usage entry point beside Threads, transcript navigation, and Share when a task session is selected.
- Remove the footer entry point and its reserved space.
- Keep the localized accessible name, desktop tooltip, keyboard focus behavior, and phone drawer with a touch-sized trigger. The phone path applies at the `isMobile` breakpoint even when the pointer is fine; coarse pointers retain the drawer at wider breakpoints.
- Keep missing, pending, error, and session-switch behavior without an empty toolbar gap or horizontal overflow.

### Out of scope

- Usage accounting, API routes, pricing, or the native Codex feature flag.
- Permission approval actions or button sizing. Desktop fine-pointer Approve/Deny buttons already use the prior 24px `h-6` style; the later responsive classes affect narrow or coarse-pointer contexts.

## Technical approach

`MessageListFooter` currently mounts `ConversationUsageDisplay` after the transcript. Remove that mount and pass the existing task/session IDs from `ChatStatusBar` to one Usage control in its right-hand icon cluster. Keep the control's `useConversationUsage` hook and `conversation-usage-trigger` test ID. Update right-control visibility so the Usage component mounts for a selected task session even before other optional controls appear, while an empty result leaves no visible spacer.

Render `IconChartBar` alone. Reuse the localized `task:conversationUsage.open` label for `aria-label` and the fine-pointer tooltip. Match the neighboring compact icon controls on desktop; retain a 44px trigger on phone and coarse pointers. Keep the existing popover and `MobilePickerSheet` content and focus return. The phone status row can wrap at group boundaries while remaining within the viewport.

No backend, feature-flag, translation-catalog, or permission-row changes are needed.

## ASCII UI preview

UI-01: Selected chat session with recorded usage. The icons shown for Threads, navigation, and Share are conditional; their order illustrates the control group. Bracketed names are annotations, not visible button text. The usage disclosure content does not change.

```text
Desktop before
  transcript footer                                  [stats Usage]
  above composer        [Threads] [start] [last] [Share] [Implement ->]

Desktop after
  transcript footer                                  (no Usage control)
  above composer        [Threads] [start] [last] [stats] [Share] [Implement ->]
                                                          ^ icon only; tooltip: Usage

Phone after
  above composer        [Threads] [start] [last] [stats] [Share]
  on tap [stats]       inset Usage drawer with the existing details and Close
```

The icon and neighboring actions remain reachable if the row wraps. The phone stats target is at least 44 by 44 CSS pixels; the desktop icon matches adjacent compact controls. This preview maps to `AC-COSTS-CONVERSATION-USAGE-003.3` and `.5`.

## Tests

- `components/task/chat/chat-status-bar.test.tsx`: a selected task/session passes its identity to the status-row Usage control; the empty right-control wrapper has no children and collapses.
- `components/task/chat/conversation-usage-display.test.tsx`: the desktop trigger is icon-only, keeps the localized accessible name, and retains the touch drawer behavior.
- Existing footer tests retain coverage of footer status and recovery behavior. The desktop and mobile E2E tests verify the single Usage trigger is in the status row, so no footer duplicate remains.

## E2E tests

- `tests/chat/conversation-usage.spec.ts` (Chromium): confirm the single icon lives in the status row, measures 24 by 24 CSS pixels on desktop, shows the Usage tooltip on hover and focus, opens the desktop popover by keyboard, and returns focus after closing. At a 500px fine-pointer viewport, confirm the trigger measures at least 44 by 44 and opens the phone drawer.
- `tests/chat/mobile-conversation-usage.spec.ts` (mobile Chrome): confirm the icon is in the composer status row, measures at least 44 by 44 CSS pixels, opens the same drawer, returns focus, stays within the row, and does not create horizontal page overflow.

## Work orders

- [x] [Task 01: Move Usage into chat status row](task-01-move-usage-control.md)

## Verification results

Passed. The focused component suite passed 16 tests across three files, web typecheck passed, and `make build-web` succeeded. The desktop Chromium Usage E2E passed both the 24px desktop popover and the 500px fine-pointer phone drawer; the mobile Chrome test passed with a 44px target and the existing inset drawer behavior. `git diff --check` passed. A phone screenshot confirmed the existing inset drawer layout and single scroll owner.

## Risks

- The status row includes conditional controls and may wrap on narrow screens. The mobile E2E verifies that the Usage trigger remains within the row and the page has no horizontal overflow.
- The footer and status row have different mount lifecycles. `useConversationUsage` scopes summary and detail state by task/session identity, and the status bar passes the selected IDs.
