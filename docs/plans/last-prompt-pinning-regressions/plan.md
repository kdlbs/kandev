---
spec: docs/specs/ui/requirements/last-prompt-pinning-regressions.md
created: 2026-07-28
status: completed
---

# Implementation Plan: Repair last-prompt transcript pinning

## Overview

Correct the visibility predicate so the desktop pinned bar opens only after the last prompt fully leaves the transcript viewport, rather than when its top first clips. Replace the navigation glyph and derive the expand control from actual two-line overflow. Keep the persisted preference and mobile fallback unchanged, then verify the repaired behavior in desktop and mobile Playwright flows and attach hosted PR screenshots.

## Root cause

A top-only predicate opens the bar while some prompt lines remain visible. The tab strip is already outside the scrollport: live geometry shows it ends at y=80 and the bar correctly pins at y=80. The threshold must therefore require no prompt/scrollport intersection, not account for an unmeasured tab height. `AnchoredLastPromptBar` currently uses an 80-character/newline heuristic, which does not reflect rendered wrapping or clipping. `TranscriptScrollButton` imports `IconArrowBackUp` rather than the requested straight up arrow.

## Frontend

### Transcript edge tracking

- `apps/web/components/task/chat/message-list-shared.tsx`: change the native geometry predicate to report the last prompt past only when it is fully outside the scrollport.
- `apps/web/components/task/chat/message-list-native.tsx`: retain the shared fully-out-of-view predicate.
- `apps/web/components/task/chat/message-list-virtuoso.tsx`: inspect the mounted prompt DOM geometry for the same threshold and use both range bounds only when virtualization has unmounted it.

### Pinned prompt controls

- `apps/web/components/task/chat/scroll-to-last-prompt-button.tsx`: use `IconArrowUp` for both transcript navigation controls.
- `apps/web/components/task/chat/anchored-last-prompt-bar.tsx`: constrain collapsed text to two lines; track real overflow from its `scrollHeight` and `clientHeight` on layout/resize; render expand only when content is clipped. Preserve inert hidden state and the bounded expanded scroll area.

### Directional edge state (task 04 follow-up)

Task 01's fully-out-of-view predicate is direction-blind: it also fires while the last prompt sits below the viewport (not yet reached, e.g. browsing earlier history), incorrectly opening the anchored bar there too. Task 04 replaces it with `resolveLastPromptEdge`/`resolveVirtuosoEdgeState` returning `above | below | visible`. The anchored bar now opens only for `above`; the always-on scroll button stays eligible for both `above` and `below` but flips `IconArrowUp` → `IconArrowDown` while `below`, without changing its scroll-to-last-prompt action. See `apps/web/components/task/chat/message-list-shared.tsx`, `message-list-virtuoso-edges.ts`, `scroll-to-last-prompt-button.tsx`, `transcript-nav-group.tsx`, `chat-input-area.tsx`, `task-chat-panel.tsx`.

### Proportional expand height and streaming-scroll resilience (task 05 follow-up)

The expanded pinned prompt used the message queue's fixed `max-h-[40rem]` cap, which is wrong for a transcript panel that can be much taller or shorter than the composer overlay it was copied from. Task 05 measures the transcript's own scroll container (`.chat-message-list`, the established scroll-container selector already used by `dockview-scroll-preserve.ts` and `use-resizable-input.ts`) via `ResizeObserver` and applies 40% of its live height as an inline `maxHeight`, falling back to `40vh` only when no such ancestor is mounted. Separately, `scroll-to-start`/`scroll-to-last-prompt` could be silently cancelled if the agent streamed a new message while the `scrollIntoView` smooth animation was still in flight: `useAutoScroll`'s follow-bottom effects and `useScrollPositionOnPrepend`'s prepend-restore effect both write `scrollTop` unconditionally on any `messages`/`items.length` change. A shared `shouldAutoScrollToBottom` predicate and a programmatic-scroll lock (`useProgrammaticScrollGuard` for the native renderer, `useProgrammaticScrollLock` gating Virtuoso's `followOutput` for the windowed renderer) now suppress both writers while a user-initiated scroll is in flight, releasing on `scrollend` or a bounded timeout and resyncing the near-bottom state from the actual scroll position before releasing. See `apps/web/components/task/chat/anchored-last-prompt-bar.tsx`, `message-list-shared.tsx`, `message-list-native-scroll.ts`, `message-list-virtuoso-scroll-lock.ts`.

### Mobile contract

Desktop stays an inline, full-width pinned transcript affordance directly below the existing dockview tab strip. Mobile retains the closest existing mobile chat composition: the transcript's status-bar action is the primary, touch-reachable entry point; no desktop bar is mounted or squeezed into the phone layout. The transcript remains the single scroll owner. Existing `mobile-last-prompt-scroll.spec.ts` is the mobile regression path.

## Tests

- **What:** a partially visible last prompt does not open the bar; a prompt fully above the viewport does; a prompt fully below the viewport does not (task 04).
  **File:** `apps/web/components/task/chat/message-list-shared.test.tsx`, `message-list-virtuoso-edges.test.ts`.
  **How:** directional geometry/range-mapping unit tests (`resolveLastPromptEdge`, `resolveLastPromptControls`, `resolveVirtuosoEdgeState`) that fail under a direction-blind predicate.
- **What:** the expand decision reflects measured clipping rather than text length.
  **File:** `apps/web/components/task/chat/anchored-last-prompt-bar.test.tsx`.
  **How:** component test with controlled measurements/ResizeObserver for fitting and clipped two-line prompt states.
- **What:** straight upward arrow remains the accessible transcript navigation icon; it flips to a downward arrow while the last prompt sits below the viewport, and the click action is unaffected either way (task 04).
  **File:** `apps/web/components/task/chat/scroll-to-last-prompt-button.test.tsx`.
  **How:** rendered icon assertion matching existing button test conventions.
- **What:** the expanded pinned prompt's max-height is 40% of the transcript container's actual height (proportional, not a fixed rem value), with a documented viewport-relative fallback when no transcript ancestor is mounted (task 05).
  **File:** `apps/web/components/task/chat/anchored-last-prompt-bar.test.tsx`.
  **How:** component tests asserting the computed inline `maxHeight` at two different mocked container heights, plus the no-ancestor fallback.
- **What:** `useAutoScroll`'s and `useScrollPositionOnPrepend`'s forced-bottom-scroll decisions are suppressed while a programmatic scroll-to-start/scroll-to-last-prompt is in flight (task 05).
  **File:** `apps/web/components/task/chat/message-list-shared.test.tsx`.
  **How:** `shouldAutoScrollToBottom` truth-table unit tests covering the near-bottom, locked, and pending-restore combinations.

## E2E Tests

- **Desktop:** `apps/web/e2e/tests/chat/last-prompt-scroll.spec.ts` — keep the anchored bar hidden while part of the last-prompt row remains visible, then assert it appears only after the row fully leaves above; verify the control's accessible name and the collapsed/expanded prompt states; verify the bar stays closed and the scroll button points down while the last prompt sits below the viewport (task 04); verify scroll-to-start drains older pages until it reaches the true first prompt rather than a partial-page boundary; verify scroll-to-start still lands on target when a message streams in immediately after the click (task 05).
- **Mobile:** `apps/web/e2e/tests/chat/mobile-last-prompt-scroll.spec.ts` — with the setting enabled, assert the upward scroll-to-last-prompt action works while no anchored bar is mounted.

## Screenshot delivery

Capture final Task Actions and desktop pinned-bar screenshots in `apps/web/.pr-assets/`. Attach them through the authenticated GitHub PR composer so #1999 contains hosted `github.com/user-attachments` URLs; do not commit PR assets.

## Implementation waves

1. [task-01-transcript-threshold-and-controls](task-01-transcript-threshold-and-controls.md) — sequential.
2. [task-02-pinned-prompt-overflow](task-02-pinned-prompt-overflow.md) — sequential; depends on task 01 because it shares the bar's rendered test surface.
3. [task-03-e2e-and-pr-assets](task-03-e2e-and-pr-assets.md) — sequential; depends on tasks 01–02.
4. [task-04-directional-edge-state](task-04-directional-edge-state.md) — sequential; depends on tasks 01–03. Follow-up fixing the "fully out of view" predicate's direction-blindness, discovered after task 03 landed.
5. [task-05-proportional-height-and-streaming-scroll](task-05-proportional-height-and-streaming-scroll.md) — sequential; depends on tasks 01–04. Follow-up making the expanded pinned prompt's height proportional to the transcript panel and hardening scroll-to-start/scroll-to-last-prompt against streamed content arriving mid-scroll.
