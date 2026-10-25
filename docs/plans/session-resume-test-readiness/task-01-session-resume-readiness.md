---
id: "01-session-resume-readiness"
title: "Harden session-resume readiness"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-resume-runtime-recovery.md"
---

# Task 01: Harden session-resume readiness

Replace the flaky lifecycle assumption in the existing session-resume E2E test
with a bounded poll of the created session's persisted state. Keep the UI
assertions that prove the task remains in the Turn Finished bucket across a
backend restart and automatic resume.

## Acceptance

- The test waits for the created session to reach `WAITING_FOR_INPUT` before
  asserting the initial Turn Finished row, without fixed sleeps or larger
  locator timeouts.
- The test still verifies transcript persistence, post-restart review state,
  absence from Backlog and Running, and final Turn Finished placement through
  the UI.
- The focused test passes with retries disabled under the managed E2E runner and
  under four CI-style repetitions in one isolated worker.

## Files likely touched

- `apps/web/e2e/tests/session/session-resume.spec.ts`

## Dependencies

None.

## Parallelism

`sequential`.

## Inputs

- Spec scenario: completed-turn response persistence and backend restart resume.
- Plan section: **E2E test synchronization**.
- `apps/web/e2e/references/ui-state-and-cleanup.md`: agent output is not
  lifecycle readiness; poll the exact session state before follow-up assertions.
- `apps/web/e2e/helpers/api-client.ts`: `createTaskWithAgent` and
  `listTaskSessions` response shapes.

## TDD sequence

1. Preserve the CI failure evidence as the RED signal: the existing test can
   render the response and idle composer while the sidebar row has no finished
   icon.
2. Add the exact-session `WAITING_FOR_INPUT` readiness poll.
3. Require the durable `Resumed agent Mock` boot message before the post-restart
   poll so the assertion proves automatic resume ran.
4. Run the focused test with retries disabled, then run the repeated isolated
   worker command.
5. Re-run the final changed test after any formatting or selector adjustment.

## Verification

Run from `apps/web` after the workspace dependencies are installed:

```bash
pnpm e2e:run tests/session/session-resume.spec.ts -- --grep 'task stays in Turn Finished section after backend restart and agent resume' --retries=0 --workers=1
pnpm e2e:docker --no-build -- --repeat-each=4 --workers=1 --retries=0 tests/session/session-resume.spec.ts:121
```

The managed runner must rebuild the production Go-served Vite assets before the
first command. Confirm each command discovers the intended test and leaves no
generated E2E artifacts in the diff.

## Risks

- A poll over `sessions[0]` could observe the wrong session. Match the created
  session ID when it is available and fail clearly if it is missing.
- The post-restart `WAITING_FOR_INPUT` poll must follow the durable resumed-agent
  boot message, or it could accept the pre-restart idle state.
- Polling only the API could hide a frontend hydration issue. Keep the existing
  sidebar locator assertions as the user-visible proof.

## Output contract

Report the exact files changed, the bounded poll condition, RED evidence, both
verification commands and outcomes, generated-artifact cleanup, and any
remaining CI-shard risk. Update this task's `status` and `## Results`, then
synchronize the plan checkbox and `## Verification Results`.

## Results

- Added an exact-session poll for `WAITING_FOR_INPUT` before the initial
  Turn Finished assertion and again after automatic resume.
- Added a durable `Resumed agent Mock` boot-message assertion before the
  post-restart poll, so the final state check cannot accept the pre-restart
  idle state.
- Preserved the transcript, restart, Backlog, Running, and final Turn Finished
  UI assertions.
- The original CI failure showed the response and idle composer before the
  sidebar received the settled lifecycle state. The new poll synchronizes the
  UI assertion with that persisted state instead of adding a sleep or larger
  locator timeout.
- Focused managed E2E run after review fix: passed, 1 test in 8.3s.
- Four-repeat single-worker E2E run after review fix: passed, 4 tests in 31.2s.
- Web typecheck, Prettier check, and `git diff --check`: passed.
- No generated E2E artifacts remain in the worktree.
