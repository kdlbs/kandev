---
id: "04-prove-responsive-recovery"
title: "Prove responsive npm recovery"
status: done
wave: 4
depends_on: ["03-present-npm-recovery"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 04: Prove responsive npm recovery

Exercise the shipped Kanban and Office recovery surfaces in real browser
layouts.

- **Acceptance:** Desktop Kanban shows npm-specific copy, collapsed details,
  one **Retry runtime** action, and the expected recovery request.
- **Acceptance:** A phone viewport keeps the inline card within the chat, has no
  horizontal overflow, uses a touch target of at least 44 px, and opens no
  nested overlay.
- **Acceptance:** Office restores the specialized card from persisted
  `last_agent_error` metadata after reload and offers the same single action.
- **Verification:** Build once, add the browser scenarios, and run:

  ```bash
  make build-web
  make build-backend
  cd apps/web
  pnpm e2e:run --no-build tests/session/managed-runtime-npm-recovery.spec.ts tests/office/managed-runtime-npm-recovery.spec.ts
  pnpm e2e:run --no-build --project mobile-chrome tests/session/mobile-managed-runtime-npm-recovery.spec.ts
  ```

- **Files likely touched:**
  `apps/web/e2e/tests/session/managed-runtime-npm-recovery.spec.ts`,
  `apps/web/e2e/tests/session/mobile-managed-runtime-npm-recovery.spec.ts`, and
  `apps/web/e2e/tests/office/managed-runtime-npm-recovery.spec.ts`.
- **Dependencies:** Task 03.
- **Parallelism:** Can run in parallel with Task 05 after Task 03.
- **Inputs:** The shipped structured failure metadata and shared recovery UI.
- **Output contract:** Report files changed, desktop and mobile command results,
  viewport and touch-target evidence, overflow evidence, Office reload evidence,
  and synchronized task and plan status.
