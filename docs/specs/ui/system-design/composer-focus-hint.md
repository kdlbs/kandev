---
status: current
system: ui
requirements:
  - REQ-UI-COMPOSER-FOCUS-HINT-001
---

# Composer Focus Hint System Design

## Purpose and boundaries

The shared composer owns presentation across task chat and quick chat. This
change affects neither session state nor shortcut dispatch.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-UI-COMPOSER-FOCUS-HINT-001 | Components and control flow |

## Components and control flow

`shouldShowChatFocusHint` in `use-chat-input-container.ts` currently derives
state eligibility without viewport information. Keep that state policy intact.
In `ChatInputBody`, read `isMobile` from `useResponsiveBreakpoint` and derive
one effective visibility value: `showFocusHint && !isMobile`. Use it for both
`ChatInputFocusHint.visible` and the editor's conditional `pr-28` class.
This keeps direct consumers and viewport transitions consistent and avoids
hiding only the hint while retaining wasted editor space.

## Mobile composition

Use the existing inline composer and mobile slash-command flow as the nearest
exemplar (`e2e/tests/chat/mobile-slash-command-composer.spec.ts`). The editor and
send action retain their order, touch targets, scroll owner, dynamic viewport,
and safe-area behavior. No additional drawer, navigation, or persisted state is
needed. Width below 768px defines phone scope even with a fine pointer.

## Compatibility and recovery

The existing reactive breakpoint hook updates on boundary changes without
remounting the editor. Wider touch devices retain current behavior. No new
translation, API, persistence, permission, logging, or metrics contract is
introduced. Existing editor focus and submission handlers are unchanged.

## Verification

Component tests drive the responsive hook through mobile and wider states,
checking hint absence and removal of reserved padding together. Existing state
predicate tests preserve wider eligibility. Playwright verifies phone tapping,
slash-command entry, submission, and wider-screen hint behavior.

## Implementation Plans

- [Mobile composer focus hint](../../../plans/mobile-composer-focus-hint/plan.md)
