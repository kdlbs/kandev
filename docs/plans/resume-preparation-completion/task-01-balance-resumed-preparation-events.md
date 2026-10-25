---
id: "01-balance-resumed-preparation-events"
title: "Balance resumed preparation events"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.9
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
---

# Task 01: Balance Resumed Preparation Events

## Summary

Make worktree ACP resume preparation publish its terminal event and prove that
desktop and mobile chat return to their actual idle state. Preserve the current
event-free fast path for ACP resumes that skip preparation.

## In scope

- Start with a failing lifecycle regression that launches a worktree resume with
  an ACP session ID and observes preparation events through the real event bus.
- Gate terminal-event suppression with the same preparation decision used by
  environment and runtime progress.
- Cover success, failure, and skipped-preparation behavior where the existing
  test harness supports them without duplicating launch setup.
- Extend the existing desktop and mobile session-resume recovery scenarios with
  post-idle assertions for stale preparation and background-work status.
- Record final command results in this work order and `plan.md`.

## Out of scope

- Frontend state-model or component changes unless the balanced backend event
  does not satisfy the documented contract.
- New responsive composition, copy, or localization.
- ACP protocol changes, branch recovery policy, or task-state transitions.

## Acceptance

- A worktree ACP resume that emits preparation progress emits exactly one
  terminal preparation result for the same launch attempt.
- An ACP resume that skips preparation continues to emit no preparation
  progress or completion.
- After successful recovery settles at `WAITING_FOR_INPUT`, desktop and mobile
  chat show neither stale workspace preparation nor false background work.

## Verification

```bash
# From apps/backend:
rtk go test ./internal/agent/runtime/lifecycle -run 'TestLaunch_(WorktreeResumePublishesPrepareCompleted|ResumeWithoutPreparationPublishesNoPrepareEvents|PublishesPrepareCompletedAfterRuntimeProgress|PublishesPrepareCompletionOnLegacyRouteEnvError)$' -count=1

# From apps/web:
rtk pnpm e2e:run tests/session/session-resume-recovery.spec.ts -- --retries=0
rtk pnpm e2e:run --project mobile-chrome tests/session/mobile-session-resume-recovery.spec.ts -- --retries=0

# From the repository root:
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch_prepare_events_test.go`
- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/tests/session/session-resume-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-resume-recovery.spec.ts`
- `docs/plans/resume-preparation-completion/plan.md`
- `docs/plans/resume-preparation-completion/task-01-balance-resumed-preparation-events.md`

## Dependencies

None.

## Risks

- The completion predicate can drift from progress gating if it duplicates the
  resume rule instead of calling `shouldPrepareEnvironment`.
- The existing E2E recovery flow performs real Git branch replacement and is
  slower than a store-only test, but it is the closest production path to PR
  #3216 and the reported incident.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001` and AC 001.9.
- `docs/specs/agents/system-design/agent-resume-runtime-recovery.md`.
- PR #3216 and its `session-resume-recovery` unit and Playwright patterns.
- Incident diagnostics for session `9dd7b17f-0c66-4c73-8e1e-ff5c7eb67c35`.

## Results

- Added a real launch-path regression that reproduced the missing terminal
  event for an ACP worktree resume before the fix.
- Reused `shouldPrepareEnvironment` when deciding whether to suppress terminal
  events, which preserves the ordinary ACP resume fast path.
- Added launch-level coverage for skipped preparation and terminal failure, and
  extended desktop/mobile branch-recovery E2E tests with idle-state assertions.
- All commands in **Verification** pass.
