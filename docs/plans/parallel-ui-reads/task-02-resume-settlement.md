---
id: "02-resume-settlement"
title: "Settle background work before resume"
status: done
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-010
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-010.1
  - AC-AGENTS-BACKGROUND-WORK-010.5
system_design:
  - ../../specs/agents/system-design/background-work.md
---

# Task 02: Settle background work before resume

## Summary

Repair the background-process regression found during PR CI. Resume can hold
the lifecycle lock while the old execution's stopped handler waits to settle
its jobs. A successor then makes the liveness check refuse settlement, leaving
the predecessor's jobs running in the projection.

## Scope

After successful stale-runtime cleanup, call the existing orchestrator
settlement while resume still holds the lifecycle lock and before the successor
launches. Cover both terminal preflight and stale-launch fallback. Keep the
existing liveness, retained-runtime, and failed-probe guards.

## Files

- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_cleanup.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_settlement_test.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/web/e2e/tests/task/background-work-process-end.spec.ts`
- `apps/web/e2e/tests/task/mobile-background-work-process-end.spec.ts`
- `docs/public/tasks-and-workflows.md` (how-to guide).

## Validation

Run the executor package tests and the existing background-settlement tests.
Prove the new regression fails when the cleanup callback is removed. Replay the
failed CI test group with one worker, two CPUs, and retries disabled. Run
changed-file Go lint, documentation catalog validation, and specification lint.

The desktop and phone process-end specs explicitly disable retries. Focused
commands, run from the repository root:

```bash
(cd apps/backend && GOMAXPROCS=2 go test -trimpath -p=2 ./internal/orchestrator/executor -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -trimpath -p=2 ./internal/orchestrator \
  -run 'Test(OwnedTeardown|.*BackgroundWork|IdleReclaimSettles|StartupSettlement)' -count=1)
pnpm --dir apps/web e2e:run --host --project chromium -- \
  --project=mobile-chrome e2e/tests/task/background-work-process-end.spec.ts \
  e2e/tests/task/mobile-background-work-process-end.spec.ts \
  e2e/tests/session/mobile-acp-prompt-error-continuity.spec.ts --retries=0
```

## Evidence and results

The CI background-process test failed after resume with two active workloads.
The same failure reproduced twice in its CI ordering. The captured database
kept both workloads on the predecessor's turn while a successor execution was
ready. The log showed the stopped handler waiting behind resume. Isolated
tests passed. The fix changes lifecycle ordering and keeps the existing
timeouts; both process-end specs now require a first-attempt pass.

The new regression covers successful and failed cleanup at both resume paths.
Removing the callback made both successful-cleanup cases fail. The executor
package, background-settlement tests, and changed-file Go lint passed. A fresh
production build passed 45 E2E cases: the failed CI group's 44 tests and the
phone process-end test. The run used one worker, two CPUs, and no retries.
Both process-end cases and both mobile error-continuity cases passed. Catalog validation,
specification lint, public-doc validation, and its 63 tests passed.
