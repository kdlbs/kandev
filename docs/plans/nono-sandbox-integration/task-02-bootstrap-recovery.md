---
id: "02-bootstrap-recovery"
title: "Recover failed bootstrap attempts without losing queued work"
status: pending
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005
acceptance_criteria:
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.1
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.2
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.3
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.4
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.5
system_design:
  - ../../specs/tasks/system-design/bootstrap-attempt-recovery.md
---

# Task 02: Recover failed bootstrap attempts without losing queued work

## Summary

Reproduce Task 04 on the current parent and repair the generic attempt/queue distinction.

## In scope

Use failed-attempt evidence, exact session recovery, safe error classification, and blocked queue projection. Preserve normal pending-launch behavior.

## Out of scope

No sandbox-specific recovery shortcut, live database repair, sibling queue transfer, or automatic policy change.

## Acceptance

- A failed authentication bootstrap without an executor record recovers the same session after prerequisites are corrected.
- A blocked prompt remains visible without a retry loop and dispatches once after readiness.
- Sibling success and stale completions preserve the original error and queue ownership on desktop and phone.

## UI preview

UI-04: Recovery. See the [combined preview](plan.md#ui-preview).
Desktop: session error card, blocked queue count, Retry action. Phone: full-width session card with visible Retry and queue controls.
Preserve loading, error, expired, and read-only states. Use shared domain state and localized labels.

## Verification

Commands run from the Kandev repository root unless a subshell selects the dedicated plugin checkout.
New test and runner paths below are implementation deliverables, not existing passing evidence.

```bash
(cd apps/backend && go test -trimpath ./internal/orchestrator ./internal/agent/runtime/routingerr ./internal/task/service -run 'BootstrapAttempt|QueueUserPrompt_RuntimeUnavailable|PromptTask_.*RuntimeUnavailable' -count=1)
pnpm --dir apps/web e2e:run --host --project chromium tests/task/bootstrap-attempt-recovery.spec.ts -- --retries=0
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/task/mobile-bootstrap-attempt-recovery.spec.ts -- --retries=0
```

Use TDD for changed logic. Before the first pnpm command in this workspace, run `(cd apps && pnpm install --frozen-lockfile)`.
Run each listed suite after its files exist. Do not count a zero-test filter as a pass.

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/bootstrap_attempt_recovery.go (new)`
- `apps/backend/internal/orchestrator/bootstrap_attempt_recovery_test.go (new)`
- `apps/backend/internal/agent/runtime/routingerr/startup_error_test.go`
- `apps/backend/internal/task/service/`
- `apps/web/components/task/chat/session-recovery-card.tsx`
- `apps/web/e2e/tests/task/bootstrap-attempt-recovery.spec.ts (new)`
- `apps/web/e2e/tests/task/mobile-bootstrap-attempt-recovery.spec.ts (new)`

## Dependencies

None. Execute sequentially under the plan.

## Risks

The retained build is older. Preserve already-fixed behavior if the current source passes the reproducer.

## Parallelism

`sequential`

## Inputs

- [Evidence](investigation.md).
- [Design](../../specs/tasks/system-design/bootstrap-attempt-recovery.md).

## Results

Pending. No production implementation is authorized during the design turn.
