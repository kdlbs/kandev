---
id: "04-idle-dot-e2e"
title: "Idle dot E2E"
status: complete
wave: 3
depends_on: ["02-ws-marking-hooks", "03-entry-point-dots"]
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-chat-idle-dot.md"
---

# Task 04: Idle dot E2E

Playwright coverage for the dot lifecycle on the desktop, tablet, and mobile
Quick Chat entry points.

- **Acceptance:**
  1. Chromium (desktop) `quick-chat-idle-dot.spec.ts`: no dot initially on `sidebar-quick-chat-shortcut`; after opening quick chat with the mock agent, sending `/slow 8s`, closing the dialog, and the session's `session.turn.completed` WS event firing, the dot is visible on the shortcut; reopening the dialog removes it; re-arm (spec scenario 5): arm a second completion wait, send a second message WHILE the dialog is open (the composer only accepts input while open), close the dialog while it is pending, await the completion, THEN the dot is visible again.
  2. Chromium (tablet) same lifecycle against `tablet-quick-chat-button`, using the `tabletTestPage` fixture (900×900, `hasTouch: true`) — a fine-pointer desktop context at ~1024 renders compact desktop, not the tablet header.
  3. Mobile-chrome `mobile-quick-chat-idle-dot.spec.ts` (the `mobile-chrome` project collects only `mobile-*.spec.ts`): same lifecycle against `mobile-quick-chat-button` (tap-driven, `quick-chat-close`), plus the task-switcher sheet entry — markers are ephemeral and lost on `page.goto`, so navigation must precede marker creation: seed a task (`apiClient.seedTask`, pattern `mobile-quick-chat-entry.spec.ts`), navigate to `/t/${taskId}` and wait for the session page, tap `mobile-sheet-quick-chat` (opens the Quick Chat setup dialog), select an agent, arm the waitForResponse for `POST /api/v1/workspaces/<id>/quick-chat` immediately BEFORE clicking `quick-chat-start` (the sheet tap opens setup; the POST fires on `quick-chat-start` — see `quick-chat-helpers.ts:53-56`), capture `session_id` from the response, send `/slow 8s`, close the dialog, await the completion, reopen the sheet, and assert the dot on `mobile-sheet-quick-chat`.

- **Verification:**
  ```sh
  cd apps/web && pnpm e2e:raw -g "quick chat idle dot" \
    && pnpm e2e:raw --project=mobile-chrome -g "quick chat idle dot"
  ```
  (Fresh worktree bootstrap when deps are missing: `cd apps && pnpm install --frozen-lockfile` first.)

- **Files likely touched:**
  - `apps/web/e2e/tests/chat/quick-chat-idle-dot.spec.ts` — new desktop/tablet spec.
  - `apps/web/e2e/tests/chat/mobile-quick-chat-idle-dot.spec.ts` — new mobile spec (name must match `mobile-*.spec.ts` for the `mobile-chrome` project); includes the `mobile-sheet-quick-chat` assertion via the task switcher sheet flow from `mobile-quick-chat-entry.spec.ts`.
  - Reuse `apps/web/e2e/tests/chat/quick-chat-helpers.ts` (`openQuickChatWithAgent`, `sendQuickChatMessage`) and `apps/web/e2e/helpers/causal-waits.ts` (`watchWs`). Capture `session_id` AND `task_id` from the `POST /api/v1/workspaces/<id>/quick-chat` response yourself (pattern: `apps/web/e2e/tests/chat/entity-reference-composer.spec.ts:201-213` — the shared helpers return only the dialog).
  - Tablet fixtures: `tabletTestPage` from `apps/web/e2e/fixtures/test-base.ts` (or a context with viewport <1024 and `hasTouch: true`).

- **WS arming order (required, see `causal-waits.ts`):** call `watchWs(page)` BEFORE the first `page.goto()`, then arm each `const completed = ws.waitForEvent("session.turn.completed", { where: (p) => p.session_id === sessionId })` BEFORE the message send that triggers it. `watchWs` buffers nothing and the gateway has no replay on subscribe — a wait armed after the event fires times out. First cycle: send `/slow 8s`, close the dialog while `completed` is pending, then `await completed` and assert the dot. Re-arm cycle: reopen (dot clears), arm a fresh `waitForEvent`, send the second message while the dialog is open, close the dialog, `await completed`, assert the dot again.
- **Scoped dot locators (required):** `quick-chat-unseen-dot` is rendered by all five entry points simultaneously once a marker is active, so every assertion must scope it under the entry under test — e.g. `testPage.getByTestId("sidebar-quick-chat-shortcut").getByTestId("quick-chat-unseen-dot")` — never a page-level `getByTestId("quick-chat-unseen-dot")` (Playwright strict mode). Assert each surface individually where the spec requires it.
- **Session capture:** capture `session_id` from the `POST /api/v1/workspaces/<id>/quick-chat` response for the WS predicates (see `apps/web/lib/api/domains/workspace-api.ts`; the response also carries `task_id`). In the mobile sheet flow the quick chat is started from the sheet, so the capture must wrap that launch, and the `/t/${taskId}` navigation must happen BEFORE any marker is created (markers are ephemeral, lost on `page.goto`).

- **Dependencies:** tasks 02 and 03 (the dot must appear from real WS events).
- **Parallelism:** sequential.

- **Inputs:**
  - Spec: Scenarios 1, 2, 3, 4, 5.
  - Plan: "E2E Tests" section.
  - Existing patterns: `apps/web/e2e/tests/chat/message-queue.spec.ts` (`/slow` command + `Agent is (starting|running)` status wait), `apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts` (mobile tap + `quick-chat-close`), `apps/web/e2e/playwright.config.ts` (project `testMatch` conventions), `apps/web/e2e/README.md` (project selection, causal waits).

- **Output contract:** summary, files changed, commands + outcomes, blockers, risks; update task + plan statuses in the same conversation.

## Results

Added desktop, tablet, mobile-header, and mobile task-switcher lifecycle coverage. Chromium
and mobile-chrome targeted E2E runs passed.
