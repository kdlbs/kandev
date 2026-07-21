---
spec: docs/specs/ui/user-message-navigation.md
created: 2026-07-21
status: done
---

# Implementation Plan: User Message Navigation

## Overview

Build a renderer-neutral navigation controller and a shared floating rail first. Then connect the native and Virtuoso scroll adapters, preserving the existing 20-row cursor pagination and explicit fallback, before proving long-history behavior on desktop and mobile. No backend change is required: `useLazyLoadMessages` already prepends older pages and exposes `hasMore`.

## Frontend

### Navigation Controller

- `apps/web/hooks/use-message-navigation.ts`: replace the per-row neighbor hook with list-level stop/origin state and generation-cancelled `goPrevious`/`goNext` actions. Derive stops from rendered `RenderItem` user messages, retain message IDs across prepends, load older pages until a stop appears, and expose boundary/busy/highlight state.
- `apps/web/hooks/use-message-navigation.test.ts`: cover chronological stop extraction, viewport-origin changes, multi-page loading, no-progress/error handling, known boundaries, duplicate activation, and session-change cancellation.
- Keep `useLazyLoadMessages`'s `loadMore(): Promise<number>` contract and 20-row request unchanged unless implementation proves a small result-shape extension is necessary; search and reverse-history callers must remain compatible.

### Shared Rail And Message Actions

- Add `apps/web/components/task/chat/user-message-navigation-rail.tsx`: render the vertical up/down icon rail with accessible names, busy/disabled states, fine-pointer hover/focus disclosure, coarse-pointer persistence, 44px touch targets, and safe-area-aware positioning.
- Add `apps/web/components/task/chat/user-message-navigation-rail.test.tsx`: verify semantics, pointer-mode visibility classes, activation, busy state, and 44px geometry contract.
- `apps/web/components/task/chat/messages/chat-message.tsx` and `message-actions.tsx`: remove `useUserMessageNavigation`, `showNavigation`, left/right buttons, and navigation callbacks while preserving copy/raw/debug/timestamp actions.
- Update `apps/web/components/task/chat/messages/chat-message.test.tsx` and add or update `message-actions.test.tsx` for the removed per-message controls.

### Renderer Integration

- `apps/web/components/task/chat/message-list-shared.tsx`: define shared user-stop-to-render-item mapping and navigation/scroll adapter types; give user prompt wrappers stable message IDs for centering and highlighting.
- `apps/web/components/task/chat/message-list-native.tsx`: derive the viewport origin from scroll geometry, center the destination DOM row, replay the existing `search-flash` emphasis, and mount the rail outside the scrolling rows.
- `apps/web/components/task/chat/message-list-virtuoso.tsx`: derive origin from Virtuoso's visible range, use `scrollToIndex(..., align: "center")`, wait for the destination row to mount before highlighting, and mount the same rail outside virtualized rows.
- `apps/web/components/task/chat/messages/turn-group-message.tsx`: expose stable message anchors if a rendered user prompt is nested in a group.
- Add focused `message-list-native.test.tsx` and `message-list-virtuoso.test.tsx` coverage; extend `message-list-shared.test.tsx` for stop mapping and keep the **Load older messages** fallback assertion.

## Mobile Contract

- **Desktop outcome:** a compact right-edge rail appears on chat hover or focus and jumps between user prompts, centering them where the scroll range allows and boundary-clamping the oldest/newest prompt.
- **Mobile entry point:** the same rail is directly visible in task chat; no hover, long press, drawer, or menu is required.
- **Nearest exemplars:** `apps/web/components/task/chat/chat-input-toolbar.tsx` supplies `useResponsiveBreakpoint` pointer branching; `voice-input-button.tsx` supplies coarse-pointer touch sizing.
- **Hierarchy and surface:** direct viewport navigation is appropriate for this frequent two-action control; a drawer would add unnecessary interaction depth. Messages remain primary and the rail stays secondary.
- **Scroll and geometry:** `SessionPanelContent` remains the only vertical scroll owner; the rail respects right safe area, mobile content clears it, targets are at least 44px, and the document has no horizontal overflow.
- **Shared logic:** stop selection, paging, boundary, busy, and highlight state are shared. Native DOM scroll and Virtuoso index scroll are presentation adapters only.
- **Proof:** `mobile-user-message-navigation.spec.ts` loads an older user prompt across several pages with one touch action, then navigates down, while asserting containment and no horizontal overflow.

## Tests

- **What:** stop ordering, center-origin selection, boundary state, pagination loop, cancellation, and failure/no-progress behavior. **File:** `apps/web/hooks/use-message-navigation.test.ts`. **How:** Vitest hook tests with controlled page promises and session rerenders.
- **What:** accessible rail behavior for fine and coarse pointers, busy state, and removed per-message arrows. **Files:** `user-message-navigation-rail.test.tsx`, `chat-message.test.tsx`, `message-actions.test.tsx`. **How:** Testing Library component tests.
- **What:** identical destination centering/highlighting through native DOM and Virtuoso index adapters. **Files:** `message-list-native.test.tsx`, `message-list-virtuoso.test.tsx`, `message-list-shared.test.tsx`. **How:** component tests with mocked scroll geometry/Virtuoso handle.

## E2E Tests

- **Scenario:** one up action crosses multiple 20-row pages to an older user prompt, then down returns to the newer prompt without another fetch. **File:** `apps/web/e2e/tests/chat/user-message-navigation.spec.ts`. **Verify:** centered-or-boundary-clamped highlighted markers, disabled boundaries, hover/focus disclosure, and the retained fallback.
- **Scenario:** the same long-history navigation works by touch. **File:** `apps/web/e2e/tests/chat/mobile-user-message-navigation.spec.ts`. **Verify:** persistent rail, 44px controls, readable content, viewport containment, and no horizontal overflow.
- Exercise `?renderer=native` and `?renderer=virtuoso` in desktop coverage so renderer parity is explicit.

## Implementation Waves

Wave 1 (parallel):

- [x] [Task 01 - Navigation controller](task-01-navigation-controller.md)
- [x] [Task 02 - Floating rail and message actions](task-02-floating-rail.md)

Wave 2:

- [x] [Task 03 - Renderer integration](task-03-renderer-integration.md)

Wave 3:

- [x] [Task 04 - Desktop and mobile E2E](task-04-e2e.md)

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run hooks/use-message-navigation.test.ts components/task/chat/user-message-navigation-rail.test.tsx components/task/chat/messages/chat-message.test.tsx components/task/chat/messages/message-actions.test.tsx components/task/chat/message-list-shared.test.tsx components/task/chat/message-list-native.test.tsx components/task/chat/message-list-virtuoso.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
make build-web
cd apps/web && pnpm e2e:run tests/chat/user-message-navigation.spec.ts tests/chat/mobile-user-message-navigation.spec.ts
```

## Risks

- Prepends change render indices, so navigation must retain message IDs and let each renderer resolve the final index after loading.
- Virtuoso may not mount the destination synchronously after `scrollToIndex`; highlighting needs a bounded mount wait and cancellation on session change.
- Automatic top-sentinel pagination and rail pagination share `loadMore`; the existing synchronous loading guard must remain the single duplicate-request gate.
