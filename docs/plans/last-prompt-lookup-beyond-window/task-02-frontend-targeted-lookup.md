---
id: "02-frontend-targeted-lookup"
title: "Resolve the last prompt with a targeted fetch and wire the affordances"
status: done
wave: 2
depends_on: ["01-backend-author-type-filter"]
plan: "plan.md"
spec: "../../specs/last-prompt-pinning-regressions/spec.md"
---

# Task 02: Resolve the last prompt with a targeted fetch and wire the affordances

- **Acceptance:** The scroll-to-last-prompt control and anchored bar appear for a session whose last user prompt is older than the initially loaded window (task description only, 200 seeded agent messages), with no multi-page background drain. While the prompt is resolved but not mounted and `hasMore` is true, the effective edge is `"above"` (button points up, bar open); once the prompt is mounted, the renderer's tracked edge drives the controls. Clicking an unmounted prompt drains older pages then scrolls to it; clicking a mounted one scrolls immediately. The bounded `MAX_LAST_PROMPT_LOOKUP_PAGES` preload effect and the now-dead `shouldLoadMoreForTranscriptTarget` are removed.
- **Verification:** Unit tests first (hook, edge helper), then wiring; run `cd apps/web && pnpm exec vitest run components/task/chat/message-list-shared.test.tsx hooks/domains/session/use-last-user-message.test.ts`, then `cd apps/web && pnpm run typecheck && pnpm run lint`.
- **Files likely touched:** `apps/web/lib/api/domains/session-api.ts`, `hooks/domains/session/use-last-user-message.ts` (new) + test, `components/task/task-chat-panel.tsx`, `components/task/chat/message-list-shared.tsx` + test, `components/task/chat/use-drain-older-messages.ts` (comment).
- **Dependencies:** Task 01 (backend filter must exist for the targeted fetch to work).
- **Parallelism:** sequential — frontend consumes the new backend filter.
- **Output contract:** Summary, exact files changed, targeted test result, remaining risks, and plan/task status update.
