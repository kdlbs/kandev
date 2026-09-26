---
id: "06-office-recovery"
title: "Park Office runs that require session recovery"
status: complete
wave: 6
depends_on: ["05-continuity-feedback"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-006
acceptance_criteria:
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.1
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.2
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.3
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.4
system_design:
  - ../../specs/agents/system-design/harness-session-continuity.md
---

# Task 06: Park Office runs that require session recovery

## Summary

Park Office runs that require session recovery through the canonical session recovery block.
Preserve work identity and expose operator recovery without an automatic replacement conversation.

## In scope

- Project the task-owned recovery block into an indexed Office run recovery reference. Preserve the existing run row and claim ownership.
- Exclude parked runs from timed unpark, stale-claim retries, and normal claim selection. Keep unrelated agent work eligible.
- Preserve the claimed run and working owner while execution can still be live. Release them only after authoritative reconciliation or Stop.
- Reconcile run references after restart. Preserve the final canonical admission check when projection repair is incomplete.
- Add one actionable Office inbox item and run-detail notice per block. Use direct navigation to the shared session recovery surface.
- Resume authorized pending work through the Office scheduler with unchanged provenance and a fresh admitRun budget evaluation.
- Cover configuration sync, status-only agent recovery, Mark fixed, and notification dismissal without implicit block clearance.
- Add all locale keys and desktop/mobile operator recovery tests.

## Out of scope

Automatic context continuation, global agent unpause, synthetic workflow decisions, and a new retry scheduler.

## Acceptance

- A blocked routine remains durable and visible across ticks and restart without dispatch attempts or retry-count churn.
- An operator continuation preserves run identity and cannot bypass budget, approval, or agent-status checks.
- Desktop and mobile users can reach recovery from the run/inbox. Dismissal or agent status recovery never resumes blocked work.

## Verification

Run from the repository root. Use TDD and disposable fixtures.
New tests describe required future evidence, not existing passing results.

```bash
(cd apps/backend && rtk go test -tags fts5 -race ./internal/office/... ./internal/orchestrator/executor ./internal/task/repository/... -count=1)
(cd apps/backend && rtk go run ./cmd/sqlguard ./internal)
(cd apps/backend && rtk go test -tags fts5 -race ./internal/persistence ./internal/persistence/storeconformance -count=1)
(cd apps && rtk pnpm install --frozen-lockfile)
(cd apps/web && rtk pnpm run typecheck)
(cd apps/web && rtk pnpm run i18n:zh-hant)
(cd apps/web && rtk pnpm run i18n:check)
(cd apps/web && rtk pnpm run i18n:ratchet)
(cd apps/web && rtk pnpm e2e:run --project chromium tests/office/session-recovery-required.spec.ts tests/office/budget-enforcement.spec.ts)
(cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/office/mobile-session-recovery-required.spec.ts tests/office/mobile-budget-enforcement.spec.ts)
rtk make -C apps/backend lint
(cd apps/web && rtk pnpm run lint)
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

- `session_recovery_test.go:TestOfficeSessionRecoveryParksDurably`: AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.1.
- `session_recovery_test.go:TestOfficeRecoveryPreservesRunBudgetAdmission`: AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.2.
- `session_recovery_test.go:TestOfficeRecoveryCannotBeClearedByBackgroundActors`: AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.3.
- `session-recovery-required.spec.ts` and `mobile-session-recovery-required.spec.ts`: AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.4.

Run persistence checks with a provisioned PostgreSQL database through `KANDEV_TEST_POSTGRES_DSN`.
A skipped PostgreSQL suite does not satisfy acceptance.
Office fixtures retain their existing Office feature configuration. This package introduces no continuity or delivery toggle.

## Files likely touched

- `apps/backend/internal/office/service/scheduler_integration.go`
- `apps/backend/internal/office/service/retry.go`
- `apps/backend/internal/office/service/budget_admission.go`
- `apps/backend/internal/office/repository/sqlite/`
- `apps/backend/internal/office/models/`
- `apps/backend/internal/office/configsync/`
- `apps/backend/internal/office/dashboard/service_inbox.go`
- `apps/backend/internal/office/dashboard/handler_inbox.go`
- `apps/backend/internal/orchestrator/executor/executor_office.go`
- `apps/web/app/office/agents/[id]/runs/[runId]/run-detail-view.tsx`
- `apps/web/app/office/inbox/inbox-item-row.tsx`
- `apps/web/lib/api/domains/office-extended-api.ts`
- `apps/web/lib/state/slices/office/types.ts`
- `apps/web/src/locales/`
- `apps/backend/internal/office/service/session_recovery_test.go (new)`
- `apps/web/e2e/tests/office/session-recovery-required.spec.ts (new)`
- `apps/web/e2e/tests/office/mobile-session-recovery-required.spec.ts (new)`

## Dependencies

[Task 05](task-05-continuity-feedback.md).

## Risks

- Generic retry or notification dismissal can accidentally release the recovery block.
- Recovery must preserve provenance, budget admission, and the current generation.
- A stale projection must not allow dispatch before the canonical block check.

## Parallelism

`sequential`

This work order does not authorize subagents. Preserve unrelated user edits.

## Inputs

- [Continuity design: autonomous consumers](../../specs/agents/system-design/harness-session-continuity.md#autonomous-consumers).
- [Existing Office status-only recovery](../../specs/office/system-design/agent-recovery.md).
- [Existing queue edit fencing](../../specs/ui/system-design/message-queue-edit.md).
- [Package manifest](plan.md).

## Results

Implemented Office park-and-surface recovery with durable task-owned blocks, repeated-tick
protection, preserved budget/provenance admission, and operator recovery navigation. Recovery
settles the existing block and returns the same run to the scheduler; it never launches a direct
chat-style replacement. A scheduler regression covers same-run identity and denied budget
admission. Live browser evidence remains in the final E2E gate.
