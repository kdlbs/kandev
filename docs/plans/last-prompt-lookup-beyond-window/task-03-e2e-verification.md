---
id: "03-e2e-verification"
title: "E2E red-to-green for the autonomous-session last-prompt affordances"
status: done
wave: 3
depends_on: ["02-frontend-targeted-lookup"]
plan: "plan.md"
spec: "../../specs/last-prompt-pinning-regressions/spec.md"
---

# Task 03: E2E red-to-green for the autonomous-session last-prompt affordances

- **Acceptance:** The `apps/web/e2e/tests/chat/last-prompt-multi-tab.spec.ts` "autonomous session with only its task description as a prompt still gets the last-prompt affordances" test (already failing on current code — this is the regression test) passes after tasks 01–02. It asserts, after reloading a task page for a session seeded with 200 agent messages after its task-description user message: the status-bar scroll-to-last-prompt button is visible and the anchored bar is open; clicking the button lands the transcript on the task-description prompt (older pages drained on demand). The full spec file still passes (multi-tab switching, hidden-created tab, manual-scroll preservation).
- **Verification:** `cd apps/web && KANDEV_SERVER_HOST= pnpm e2e --project=chromium tests/chat/last-prompt-multi-tab.spec.ts` (green), then full validation `cd apps/backend && PATH=/tmp/go/bin:$PATH make test lint`, `cd apps/web && pnpm exec vitest run components/task/chat/message-list-shared.test.tsx hooks/domains/session/use-last-user-message.test.ts`, `cd apps/web && pnpm run typecheck && pnpm run lint`.
- **Files likely touched:** `apps/web/e2e/tests/chat/last-prompt-multi-tab.spec.ts` (extend the autonomous test with a click-and-lands assertion).
- **Dependencies:** Tasks 01–02.
- **Parallelism:** sequential — end-to-end verification of the whole fix.
- **Output contract:** Summary, exact files changed, targeted test result, remaining risks, and plan/task status update.
