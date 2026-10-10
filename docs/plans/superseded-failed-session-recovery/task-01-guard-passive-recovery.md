---
id: "01-guard-passive-recovery"
title: "Guard passive failed-session recovery"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-001
acceptance_criteria:
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.13
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.14
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.15
system_design:
  - ../../specs/tasks/system-design/queued-session-ownership.md
---

# Task 01: Guard passive failed-session recovery

## Summary

Add the narrow failed-session eligibility predicate to status and passive launch admission.
Preserve explicit recovery and existing automatic recovery outside the selected condition.

## In scope

- Fresh inventory evaluation and guarded admission rechecks, including deferred passive replay.
- Structured suppression using existing response fields.
- Regression tests against real repository and service paths.

## Out of scope

- UI layout, general co-residency admission, npm repair, and new persistence.

## Acceptance

1. The named status/admission regression fails before the fix and passes afterward. Suppression causes no runtime start, prompt, fallback, or primary change.
2. Mixed inventories, STARTING/RUNNING siblings, missing/ambiguous ownership, read failure, and stale status are covered. All failures fail closed for candidate passive recovery.
3. Explicit resume and message-driven recovery remain eligible. Primary FAILED, non-failed, no-working-sibling, and idle-only cases retain existing behavior.

## Implementation and RED evidence

Create `session_open_failed_recovery_test.go` with the four named suites from the plan.
Reuse `seedSessionOpenRecoveryState`, `createSessionOpenRecoverySession`, and the launch-counting service pattern from `session_open_recovery_launch_test.go`.
Assert a suppressed disposition and zero provider calls, not just a helper boolean.
Use barriers for a sibling entering RUNNING after status, ownership changing before admission, and replay after deferral.
Retain queue ownership and failure identity assertions. Verify session_focus cannot bypass the guard without idle-suspension provenance.

Extract a small helper instead of expanding the already-large task operations file.
Reload session data at guarded admission. Preserve existing lock order and do not hold a lock over runtime calls.
Do not modify the executor's co-residency warning into a blocking guard.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test ./internal/orchestrator -run '^TestSessionOpenFailedRecovery' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator -run 'TestSessionOpen|TestGetTaskSessionStatus|TestResumeTaskSession' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Record the first command's behavioral RED before editing production code. Run it again for GREEN.

## Files likely touched

- `apps/backend/internal/orchestrator/session_open_failed_recovery.go` (new)
- `apps/backend/internal/orchestrator/session_open_failed_recovery_test.go` (new)
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/session_launch.go` (only if response wiring requires it)
- `apps/backend/internal/orchestrator/session_open_recovery_launch_test.go` (existing pattern; preserve prior coverage)

## Dependencies

None.

## Risks

A stale candidate snapshot can bypass the predicate. A generic sibling lock would change the approved scope.

## Parallelism

`sequential`

## Inputs

- [Design: Superseded failed conversation recovery](../../specs/tasks/system-design/queued-session-ownership.md#superseded-failed-conversation-recovery)
- [Requirements](../../specs/tasks/requirements/queued-session-ownership.md), criteria 001.13 through 001.15.
- [Decision](../../decisions/2026-10-02-superseded-failed-session-recovery.md).

## Results

Pending. The design-turn temporary diagnostic is documented in the plan; it is not permanent test coverage.
