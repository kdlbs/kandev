---
spec: docs/specs/ui/user-message-navigation.md
created: 2026-07-21
status: completed
---

# Implementation Plan: User Message Navigation

## Overview

Keep the renderer-neutral pagination and scroll adapters, but replace the floating viewport rail with directional actions owned by each user prompt. Navigation receives the clicked message ID as its explicit origin, and a shared context connects list-level state to message rows without prop-drilling through every renderer. Replace the yellow background flash with a layout-neutral, low-contrast outline. No backend change is required.

## Frontend

### Navigation Controller

- `apps/web/hooks/use-message-navigation.ts`: expose generation-cancelled `goPrevious(messageId)` and `goNext(messageId)` actions. Derive stops from rendered `RenderItem` user messages, retain the explicit origin ID across prepends, load older pages until a stop appears, and expose per-message boundary and shared busy state.
- `apps/web/hooks/use-message-navigation.test.ts`: cover chronological stop extraction, explicit origins, multi-page loading, no-progress/error handling, known boundaries, duplicate activation, and session-change cancellation.
- Keep `useLazyLoadMessages`'s `loadMore(): Promise<number>` contract and 20-row request unchanged unless implementation proves a small result-shape extension is necessary; search and reverse-history callers must remain compatible.

### Message Navigation Actions

- Remove `user-message-navigation-rail.tsx` and its viewport-level mounts, mobile clearance, and tests.
- Add a small navigation context near the message-list boundary so `MessageActions` can read shared busy/boundary state and activate previous/next from its own message ID.
- `use-message-navigation.ts`: accept an explicit origin message ID for previous/next actions; previous can continue loading older pages while next remains loaded-history only.
- `message-actions.tsx`: render Tabler up/down icon buttons only for user messages, alongside the existing copy/raw/debug/model/timestamp actions. Use the existing hover/focus disclosure on fine pointers and directly visible 44px targets on coarse pointers.
- Update controller, message action, chat message, and renderer tests for explicit row-origin behavior and the removal of the floating rail.

### Renderer Integration

- `apps/web/components/task/chat/message-list-shared.tsx`: define shared user-stop-to-render-item mapping and navigation/scroll adapter types; give user prompt wrappers stable message IDs for centering and highlighting.
- `apps/web/components/task/chat/message-list-native.tsx`: provide list-level navigation state to message actions, center the destination DOM row, and replay the outline emphasis.
- `apps/web/components/task/chat/message-list-virtuoso.tsx`: provide the same navigation state, use `scrollToIndex(..., align: "center")`, and wait for the destination row to mount before outlining it.
- `apps/web/components/task/chat/messages/turn-group-message.tsx`: expose stable message anchors if a rendered user prompt is nested in a group.
- Add focused `message-list-native.test.tsx` and `message-list-virtuoso.test.tsx` coverage; extend `message-list-shared.test.tsx` for stop mapping and keep the **Load older messages** fallback assertion.

## Mobile Contract

- **Desktop outcome:** compact up/down icons appear with the hovered or keyboard-focused user prompt's existing actions and navigate relative to that prompt.
- **Mobile entry point:** the same actions are directly visible on user prompts; no hover, long press, drawer, or menu is required.
- **Nearest exemplar:** the existing `MessageActions` row supplies desktop hover/focus disclosure and coarse-pointer persistence. Its action ordering, spacing, and icon treatment remain authoritative.
- **Hierarchy and surface:** navigation is secondary message metadata/action behavior, so inline actions are more contextual and less visually intrusive than a viewport overlay or drawer.
- **Scroll and geometry:** `SessionPanelContent` remains the only vertical scroll owner; inline actions need no safe-area overlay clearance, navigation targets are at least 44px on coarse pointers, and the document has no horizontal overflow.
- **Shared logic:** stop selection, paging, boundary, busy, and highlight state are shared. Native DOM scroll and Virtuoso index scroll are presentation adapters only.
- **Proof:** `mobile-user-message-navigation.spec.ts` loads an older user prompt across several pages with one touch action, then navigates down, while asserting containment and no horizontal overflow.

## Tests

- **What:** stop ordering, explicit row-origin selection, boundary state, pagination loop, cancellation, and failure/no-progress behavior. **File:** `apps/web/hooks/use-message-navigation.test.ts`. **How:** Vitest hook tests with controlled page promises and session rerenders.
- **What:** accessible inline action behavior, user-only rendering, busy/boundary state, and touch sizing. **Files:** `chat-message.test.tsx`, `message-actions.test.tsx`. **How:** Testing Library component tests.
- **What:** identical destination centering/highlighting through native DOM and Virtuoso index adapters. **Files:** `message-list-native.test.tsx`, `message-list-virtuoso.test.tsx`, `message-list-shared.test.tsx`. **How:** component tests with mocked scroll geometry/Virtuoso handle.

## E2E Tests

- **Scenario:** one inline up action crosses multiple 20-row pages to an older user prompt, then its inline down action returns to the newer prompt without another fetch. **File:** `apps/web/e2e/tests/chat/user-message-navigation.spec.ts`. **Verify:** centered-or-boundary-clamped outline feedback, per-row boundaries, hover/focus disclosure, no floating rail, and the retained fallback.
- **Scenario:** the same long-history navigation works by touch. **File:** `apps/web/e2e/tests/chat/mobile-user-message-navigation.spec.ts`. **Verify:** directly visible 44px inline controls, readable content, and no horizontal overflow.
- Exercise `?renderer=native` and `?renderer=virtuoso` in desktop coverage so renderer parity is explicit.

## Implementation Waves

Wave 1 (parallel):

- [x] [Task 01 - Navigation controller](task-01-navigation-controller.md)
- [x] [Task 02 - Floating rail and message actions](task-02-floating-rail.md)

Wave 2:

- [x] [Task 03 - Renderer integration](task-03-renderer-integration.md)

Wave 3:

- [x] [Task 04 - Desktop and mobile E2E](task-04-e2e.md)

Wave 4:

- [x] [Task 05 - Inline navigation actions](task-05-inline-navigation-actions.md)

## Verification

```bash
make fmt
make typecheck
make test
make lint
make build-web
cd apps/web && pnpm e2e:run tests/chat/user-message-navigation.spec.ts
cd apps/web && pnpm e2e:run --no-build tests/chat/mobile-user-message-navigation.spec.ts -- --project=mobile-chrome
```

## Risks

- Prepends change render indices, so explicit row-origin navigation must retain message IDs and let each renderer resolve the final index after loading.
- Virtuoso may not mount the destination synchronously after `scrollToIndex`; highlighting needs a bounded mount wait and cancellation on session change.
- Virtuoso live bottom-follow must be suspended while explicit navigation settles so streamed output cannot override the user's destination.
- Automatic top-sentinel pagination and inline action pagination share `loadMore`; the existing synchronous loading guard must remain the single duplicate-request gate.
