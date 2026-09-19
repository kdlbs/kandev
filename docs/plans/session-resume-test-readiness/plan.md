---
spec: docs/specs/agents/requirements/agent-resume-runtime-recovery.md
created: 2026-08-12
status: completed
---

# Implementation Plan: Session resume test readiness

## Overview

Harden the session-resume E2E regression by synchronizing the first finished-turn
assertion with the persisted session lifecycle instead of treating a visible
agent response or idle composer as lifecycle completion. Keep the backend-restart
and UI assertions intact so the test still verifies transcript persistence,
automatic resume, and the final Turn Finished bucket.

## Root cause

The mock response is persisted before the session reaches `WAITING_FOR_INPUT`.
`SessionPage.waitForChatIdle()` observes the composer and can return during that
interval, while the sidebar still classifies the task from the prior session
state. Under CI shard load, the existing 15-second locator wait expires before
the state publication reaches the sidebar. The CI error context showed the
response and Review step, but no finished-state icon on the sidebar row.

## E2E test synchronization

- In `apps/web/e2e/tests/session/session-resume.spec.ts`, retain the task
  returned by `apiClient.createTaskWithAgent` so the test can query its exact
  session.
- After the first response and `waitForChatIdle`, poll
  `apiClient.listTaskSessions(task.id)` until the created session is
  `WAITING_FOR_INPUT`. This is the lifecycle readiness condition used by the
  sidebar classifier; do not add a fixed sleep or increase a locator timeout.
- Keep the finished-turn assertion as a UI assertion after the lifecycle poll.
  Keep the backend restart, reload, transcript, no-Backlog/no-Running, and
  post-resume UI assertions unchanged in intent.
- Before the final post-resume Turn Finished assertion, use the same exact
  session-state readiness condition so the final UI check cannot race a stale
  session snapshot.

## Tests

- **Scenario:** A completed mock turn persists its response before the session
  lifecycle settles.
  **File:** `apps/web/e2e/tests/session/session-resume.spec.ts`.
  **How:** Create a task through the existing fixture API, poll the exact
  session state to `WAITING_FOR_INPUT`, then assert the task row is in the
  Turn Finished bucket.
- **Scenario:** A backend restart resumes a review-state task.
  **File:** `apps/web/e2e/tests/session/session-resume.spec.ts`.
  **How:** Reload after restart, assert the persisted transcript and immediate
  review placement, wait for the durable `Resumed agent Mock` boot message to
  prove automatic resume ran, poll the session back to `WAITING_FOR_INPUT`, and
  assert the task remains Turn Finished without Backlog or Running placement.

## Verification Results

- `cd apps/web && pnpm e2e:run tests/session/session-resume.spec.ts -- --grep
  "task stays in Turn Finished section after backend restart and agent resume"
  --retries=0 --workers=1` — passed, 1 test in 8.3s after the review fix.
- `cd apps/web && pnpm e2e:docker --no-build -- --repeat-each=4 --workers=1
  --retries=0 tests/session/session-resume.spec.ts:121` — passed, 4 tests in
  31.2s after the review fix.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm exec prettier --check
  e2e/tests/session/session-resume.spec.ts` — passed.
- `git diff --check` — passed.
- The managed E2E runner's generated artifacts were cleaned and no generated
  files remain in the worktree.
- Review fixup: the post-restart assertion now requires the durable
  `Resumed agent Mock` boot message before polling back to `WAITING_FOR_INPUT`.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Harden session-resume readiness](task-01-session-resume-readiness.md)

The task is sequential. It edits one E2E spec and must be verified against the
managed production-build runner and the CI-style repeated isolated worker.

## Risks and out of scope

- The API poll must target the created task's session, not an arbitrary session
  in the worker, or a parallel-session regression could be hidden.
- The test must continue to assert the UI. API state is only a readiness signal,
  not a replacement for the user-visible assertion.
- No production lifecycle, sidebar classifier, timeout policy, or backend API
  behavior changes are planned.
