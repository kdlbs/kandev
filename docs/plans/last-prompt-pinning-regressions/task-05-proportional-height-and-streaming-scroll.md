---
id: "05-proportional-height-and-streaming-scroll"
title: "Make the expanded prompt height proportional and scroll actions stream-safe"
status: done
wave: 5
depends_on: ["01-transcript-threshold-and-controls", "02-pinned-prompt-overflow", "03-e2e-and-pr-assets", "04-directional-edge-state"]
plan: "plan.md"
spec: "../../specs/ui/requirements/last-prompt-pinning-regressions.md"
---

# Task 05: Make the expanded prompt height proportional and scroll actions stream-safe

- **Acceptance:** The expanded pinned prompt currently caps at a fixed `max-h-[40rem]`, copied from the message queue's composer overlay — wrong for a transcript panel that varies dramatically in height. Cap it at 40% of the transcript scroll container's (`.chat-message-list`) actual height instead, measured via `ResizeObserver` and applied as an inline `maxHeight`, falling back to `40vh` only when no such ancestor is mounted (documented, tested fallback — not a silent regression to a fixed value). Separately, `scroll-to-start`/`scroll-to-last-prompt` can be silently cancelled if the agent streams a new message while the `scrollIntoView` smooth animation is still in flight: `useAutoScroll`'s follow-bottom effects and `useScrollPositionOnPrepend`'s prepend-restore effect both force `scrollTop` unconditionally on any `messages`/`items.length` change. Introduce a shared `shouldAutoScrollToBottom` predicate and a programmatic-scroll lock — `useProgrammaticScrollGuard` for the native renderer (also gating `useScrollPositionOnPrepend`), `useProgrammaticScrollLock` gating Virtuoso's `followOutput` for the windowed renderer — so both auto-scroll writers stand down while a user-initiated scroll is in flight, releasing on `scrollend` or a bounded timeout and resyncing the near-bottom state from the actual scroll position (never assumed) before releasing.
- **Verification:** First make the proportional-height and `shouldAutoScrollToBottom` tests fail, then run `cd apps/web && pnpm exec vitest run components/task/chat/anchored-last-prompt-bar.test.tsx components/task/chat/message-list-shared.test.tsx`, `pnpm run typecheck`, `pnpm lint`, and the new Playwright cases in `e2e/tests/chat/last-prompt-scroll.spec.ts` (`pnpm e2e:run --no-build tests/chat/last-prompt-scroll.spec.ts -- --project=chromium`).
- **Files likely touched:** `apps/web/components/task/chat/anchored-last-prompt-bar.tsx`, `anchored-last-prompt-bar.test.tsx`, `message-list-shared.tsx`, `message-list-shared.test.tsx`, `message-list-native.tsx`, `message-list-native-scroll.ts` (new), `message-list-virtuoso.tsx`, `message-list-virtuoso-scroll-lock.ts` (new), `message-list-virtuoso-index.ts` (new — extracted for the `max-lines`/`max-lines-per-function` limits after the scroll-lock work grew both renderer files), `e2e/tests/chat/last-prompt-scroll.spec.ts`, `e2e/tests/chat/last-prompt-scroll-helpers.ts`.
- **Dependencies:** Tasks 01–04 (shares the anchored bar's expand affordance and the transcript edge/scroll contract those tasks established).
- **Parallelism:** sequential — shares the anchored bar's render surface and both message-list renderers' scroll-management hooks.
- **Output contract:** Summary, exact files changed, targeted test result, remaining risks, and plan/task status update.
