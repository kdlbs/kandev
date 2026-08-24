---
id: "17-verify-hardened-background-liveness"
title: "Verify hardened background liveness"
status: in_progress
wave: 17
depends_on:
  - "14-attest-background-lifecycles"
  - "15-publish-active-subagent-counts"
  - "16-sidebar-background-spinner"
plan: "plan.md"
spec: "../../specs/platform/requirements/background-work-liveness.md"
---

# Task 17: Verify hardened background liveness

- **Acceptance:** A coverage matrix proves supported Claude/Codex lifecycle
  admission, unsupported-adapter fail-closed behavior, exact session/task
  counts, Codex terminal cleanup, prompt acceptance, fresh-load state, and
  violet sidebar rendering. Backend race suites, frontend checks, and focused
  desktop/Pixel 5 busy-signal E2E pass with no stale background state after
  terminal activity or teardown.
- **Verification:** `make -C apps/backend fmt`; `make -C apps/backend test lint`;
  `cd apps/web && pnpm run typecheck`; `cd apps && pnpm --filter @kandev/web test
-- components/task/task-item.test.tsx`; `cd apps/web && pnpm e2e --project=chromium
e2e/tests/chat/busy-signal.spec.ts`; `cd apps/web && pnpm e2e --project=mobile-chrome
e2e/tests/chat/mobile-busy-signal.spec.ts`.
- **Files likely touched:** Regression/E2E corrections only, plus this task
  status. Public docs are reviewed for the new API fields; no count UI is added.
- **Dependencies:** Tasks 14-16.
- **Inputs:** All new spec scenarios, ADR 0049, task handoffs, and existing
  desktop/mobile busy-signal fixtures.
- **Worker:** Primary planner coordinates; final verification must use the
  required change-aware verifier after commit when implementation is approved.
- **Output contract:** Report the agent capability matrix, lifecycle/count/UI
  evidence, exact full verification results, residual provider limitations, and
  update only this task status.
