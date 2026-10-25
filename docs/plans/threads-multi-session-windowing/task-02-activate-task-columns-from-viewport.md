---
id: "02-activate-task-columns-from-viewport"
title: "Activate task columns from the viewport"
status: completed
wave: 2
depends_on:
  - "01-publish-compact-session-attention"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-VIEWPORT-SESSION-DELIVERY-001
  - REQ-UI-THREADS-DECK-003
acceptance_criteria:
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.1
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.2
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.3
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.4
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.5
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.6
  - AC-UI-THREADS-DECK-003.1
  - AC-UI-THREADS-DECK-003.2
  - AC-UI-THREADS-DECK-003.3
system_design:
  - ../../specs/platform/system-design/viewport-bounded-session-delivery.md
  - ../../specs/ui/system-design/threads-conversation-deck.md
---

# Task 02: Activate task columns from the viewport

## Summary

Keep stable lightweight task shells for the whole deck, but mount task-session
lists and selected conversations only in their preload and detail windows.
Prove the resource bound with 30 deterministic task shells.

## In scope

- Add a controllable board-rooted `IntersectionObserver` activation hook.
- Derive desktop preload, desktop detail, and phone nearest-snap detail sets.
- Keep `ThreadColumn` shells, ordering, flexible widths, deep-link focus, and
  horizontal scroll geometry stable.
- Mount session-list adapters only in preload and conversations only in detail.
- Unmount offscreen conversation details so standard session registration
  cleanup can unsubscribe.
- Provide a conservative no-IntersectionObserver fallback that never activates
  every conversation.
- Add RED/GREEN component tests with 30 tasks, viewport entry/exit, adjacent
  preload, deep-link target, and phone snap ownership.

## Out of scope

- Session selector UI, status icon changes, session selection policy, backend
  events, or full shell virtualization.

## Acceptance

- Initial render with 30 tasks mounts 30 small shells but session lists only for
  the preload set and chats only for the detail set.
- Desktop entry and exit update chat ownership without changing column order or
  scroll position.
- Phone settles on one detail-active task even when a neighboring snap page is
  partially visible.

## Verification

Start with a controllable observer test that fails because every thread mounts
its conversation. Then run:

```bash
(cd apps && pnpm --filter @kandev/web test -- --run components/threads/threads-board.test.tsx components/threads/thread-column-activation.test.tsx)
(cd apps && pnpm --filter @kandev/web exec eslint components/threads/threads-board.tsx components/threads/thread-column.tsx components/threads/use-thread-column-activation.ts components/threads/threads-board.test.tsx components/threads/thread-column-activation.test.tsx)
```

## Files likely touched

- `apps/web/components/threads/threads-board.tsx`
- `apps/web/components/threads/thread-column.tsx`
- `apps/web/components/threads/thread-conversation.tsx`
- `apps/web/components/threads/use-thread-column-activation.ts`
- `apps/web/components/threads/threads-board.test.tsx`
- `apps/web/components/threads/thread-column-activation.test.tsx`
- `apps/web/components/threads/AGENTS.md`

## Dependencies

- Task 01 supplies compact status for later inactive session selectors. The
  activation code must not introduce a temporary transcript-based fallback.

## Risks

- Observer cleanup during fast scroll must not leave a detached chat mounted.
- The deep-linked shell must exist before detail activation so
  `scrollIntoView` still works.
- Phone detail ownership must use the nearest snap target, not all partial
  intersections.

## Parallelism

`sequential`

## Results

Implemented and verified. Threads keeps all task shells mounted, preloads
session membership for visible and adjacent columns, and mounts conversations
only for the desktop visible set or the nearest mobile snap target. The
controllable observer suite and exact component checks pass with zero ESLint
warnings.
