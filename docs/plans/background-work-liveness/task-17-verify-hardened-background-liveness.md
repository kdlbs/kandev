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
requirements:
  - REQ-PLATFORM-BACKGROUND-WORK-LIVENESS-001
acceptance_criteria:
  - AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.1
  - AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.2
  - AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.3
  - AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.4
  - AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.5
  - AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.6
  - AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.7
  - AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.8
  - AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.9
system_design:
  - ../../specs/platform/system-design/background-work-liveness.md
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

## Restart fixture authority isolation (2026-10-27)

Both failed attempts at PR #4534 head `0bbebdcf6afa` retain traces. An exact
session GET overlaps installation of the synthetic background state while the
fixture holds only the task-session list. The full session response correctly
clears omitted runtime activity. This is competing authoritative recovery, not
a product failure to preserve background activity. Route errors during cleanup
are secondary to the failed synthetic-state assertion.

Hold both reads independently. Release the list to prove it clears the stale
projection while the individual read remains held. Release both gates in
`finally` and drain the test-owned routes with `unrouteAll({ behavior: "wait" })`.
A controlled individual-read release reproduces the original background UI
assertion failure. The corrected probe keeps it held while the list clears
activity. Existing session-slice and reconciler tests independently verify
full-snapshot omission and revision ownership. No product logic changes.

The three affected ACP restart scenarios pass three repetitions: nine cases,
retries disabled, including message preservation and the worktree Files path.
The related list/reconciler unit suites pass all 47 tests.
