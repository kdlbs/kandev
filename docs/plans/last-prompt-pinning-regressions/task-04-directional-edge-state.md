---
id: "04-directional-edge-state"
title: "Make the anchored bar and scroll button direction-aware"
status: done
wave: 4
depends_on: ["01-transcript-threshold-and-controls", "02-pinned-prompt-overflow", "03-e2e-and-pr-assets"]
plan: "plan.md"
spec: "../../specs/ui/requirements/last-prompt-pinning-regressions.md"
---

# Task 04: Make the anchored bar and scroll button direction-aware

- **Acceptance:** The "fully out of view" predicate from task 01 is direction-blind — it also fires when the last prompt sits fully *below* the viewport because the user scrolled up to browse earlier history, incorrectly opening the anchored bar in that case. Replace it with a three-state `above | below | visible` classifier (`resolveLastPromptEdge` in `message-list-shared.tsx`, generalized through `resolveVirtuosoEdgeState` for the Virtuoso strategy). The anchored bar opens only for `above`. The always-on **Scroll to last prompt** control (chat status bar and inside the anchored bar) stays eligible for both `above` and `below`, but its icon flips from `IconArrowUp` to `IconArrowDown` while `below`, without changing its action (still jumps to the top of the last prompt).
- **Verification:** First make the directional geometry/mapping tests fail, then run `cd apps/web && pnpm exec vitest run components/task/chat/message-list-shared.test.tsx components/task/chat/message-list-virtuoso-edges.test.ts components/task/chat/scroll-to-last-prompt-button.test.tsx`, `pnpm run typecheck`, `pnpm lint`, and the new Playwright case in `e2e/tests/chat/last-prompt-scroll.spec.ts` (`pnpm e2e:run --no-build tests/chat/last-prompt-scroll.spec.ts -- --project=chromium`).
- **Files likely touched:** `apps/web/components/task/chat/message-list-shared.tsx`, `message-list-shared.test.tsx`, `message-list-native.tsx`, `message-list-virtuoso.tsx`, `message-list-virtuoso-edges.ts`, `message-list-virtuoso-edges.test.ts`, `scroll-to-last-prompt-button.tsx`, `scroll-to-last-prompt-button.test.tsx`, `transcript-nav-group.tsx`, `chat-input-area.tsx`, `task-chat-panel.tsx`, `e2e/tests/chat/last-prompt-scroll.spec.ts`.
- **Dependencies:** Tasks 01–03 (shares the transcript edge contract and rendered test surface those tasks established).
- **Parallelism:** sequential — shared transcript edge contract.
- **Output contract:** Summary, exact files changed, targeted test result, remaining risks, and plan/task status update.
