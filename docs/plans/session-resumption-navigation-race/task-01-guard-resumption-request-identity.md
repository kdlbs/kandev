---
id: "01-guard-resumption-request-identity"
title: "Guard resumption request identity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.7
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
---

# Task 01: Guard Automatic Recovery by Task-Session Identity

## Summary

Reject stale automatic session-status callbacks after task navigation, even
when the old and current requests use the same session ID.

## In scope

- Add the regression test before editing production code.
- Rerender the hook with a new task ID while preserving the session ID.
- Control both status promises so the stale old-task result arrives after the
  current request has started.
- Make every request-owned setter validate the captured task ID, session ID,
  and monotonic navigation generation.
- Publish the active request identity during commit before passive effects run.
- Preserve current automatic resume, read-only fallback, and status behavior
  for the active identity.

## Out of scope

- Backend validation changes.
- Task-page routing or store synchronization changes.
- UI markup, copy, responsive behavior, and localization changes.
- New E2E coverage for a state-only fix.

## Acceptance

- A result captured for the prior task cannot set recovery error, notice,
  attempt, or session-status state after the task ID changes.
- The guard rejects the stale result when the session ID remains unchanged.
- A repeated task-session pair receives a new generation, so an earlier
  navigation cycle cannot update the returned task.
- The current task-session request still updates state normally.
- The focused hook suite, targeted lint, and web typecheck pass.

## Verification

Run from the repository root:

```bash
(cd apps && rtk pnpm --filter @kandev/web test -- --run hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-session-resumption.navigation.test.ts)
(cd apps/web && rtk pnpm exec eslint hooks/domains/session/use-session-resumption.ts hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-session-resumption.navigation.test.ts)
(cd apps/web && rtk pnpm run typecheck)
```

Before the production edit, run the focused regression by its exact test name
and record the expected failure. After the edit, rerun the exact regression and
then the complete commands above.

## Files likely touched

- `apps/web/hooks/domains/session/use-session-resumption.ts`
- `apps/web/hooks/domains/session/use-session-resumption-request-guard.ts`
- `apps/web/hooks/domains/session/use-session-resumption.test.ts`
- `apps/web/hooks/domains/session/use-session-resumption.navigation.test.ts`
- `docs/plans/session-resumption-navigation-race/plan.md`
- `docs/plans/session-resumption-navigation-race/task-01-guard-resumption-request-identity.md`

## Dependencies

None.

## Parallelism

Sequential. The test and production guard share the same hook boundary.

## Inputs

- The confirmed request sequence from the running instance.
- The amended recovery requirement and system design.
- The existing stale-callback hook tests.

## Output contract

Report the files changed, red and green test evidence, lint and typecheck
outcomes, and any remaining risk. Update this work order to `done` and the plan
to `implemented` only after all task checks pass.

## Results

- RED: the exact navigation regression failed because the stale old-task
  response set `session does not belong to task` after the correct response.
- GREEN: the exact regression passes after the task-session request-key guard.
- GREEN: the two focused session-resumption test files pass 21 tests.
- GREEN: targeted ESLint completes with no warnings or errors, web typecheck
  exits successfully, and all changed TypeScript files pass Prettier.
- GREEN: all specification files pass their linter and `git diff --check`
  succeeds.
- Mobile parity is preserved through the shared hook. No rendered or
  viewport-dependent behavior changed, so a new mobile Playwright case would
  duplicate the state-level regression without adding coverage.
