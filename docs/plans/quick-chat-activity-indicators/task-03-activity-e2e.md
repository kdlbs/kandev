---
id: "03-activity-e2e"
title: "Quick Chat activity E2E"
status: done
wave: 3
depends_on: ["02-activity-ui"]
plan: "plan.md"
spec: "../../specs/quick-chat-idle-dot/spec.md"
---

# Task 03: Quick Chat Activity E2E

- **Acceptance:**
  1. Desktop and tablet tests prove tab spinner, blue running bubble, emerald finished bubble, and clear-on-open behavior.
  2. Mobile header and task-switcher tests prove the same state sequence through touch controls.
  3. WebSocket waits are armed before each message, and all indicator locators are scoped to their entry point.

- **Verification:**
  ```sh
  cd apps && pnpm install --frozen-lockfile \
    && cd web \
    && pnpm e2e:run tests/chat/quick-chat-idle-dot.spec.ts \
    && pnpm e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-idle-dot.spec.ts
  ```

- **Files likely touched:**
  - `apps/web/e2e/tests/chat/quick-chat-idle-dot.spec.ts`
  - `apps/web/e2e/tests/chat/mobile-quick-chat-idle-dot.spec.ts`
  - `apps/web/e2e/tests/chat/quick-chat-helpers.ts` only if a shared activity locator removes duplication

- **Dependencies:** Task 02.
- **Parallelism:** sequential.
- **Inputs:** Spec scenarios, plan `E2E Tests`, and the existing `watchWs`, `openQuickChatWithAgent`, and `sendQuickChatMessage` patterns.
- **Risks:** The completion and session-state events use separate streams. Assert the running state before completion, then wait for the semantic completion before the finished assertion.
- **Output contract:** Report discovered test counts, exact command results, rendered mobile evidence, blockers, risks, cleanup, and synchronized task and plan status.

## Results

- Desktop/tablet command passed: 2 tests covering tab spinner, running state, finished state, and clear-on-open behavior.
- Mobile command passed: 2 tests covering the mobile header and task-switcher lifecycle through touch controls.
- `pnpm --filter @kandev/web run lint:e2e-sleeps` passed.
- WebSocket waits are armed before sends and all activity locators are scoped to their entry point.
- No test cleanup or runtime blockers remain for this task.
