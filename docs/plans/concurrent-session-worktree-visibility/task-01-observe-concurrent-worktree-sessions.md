---
id: "01-observe-concurrent-worktree-sessions"
title: "Observe concurrent worktree sessions"
status: planned
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004.1
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004.2
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004.3
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004.4
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
---

# Task 01: Observe Concurrent Worktree Sessions

## Summary

Record, at the two seams that start an agent process against an already
attached task workspace, that another session of the same task is already
working. Emit a structured warning and an expvar counter. Change no admission
outcome.

## In scope

- Generalize `hasOtherWorkingSessions` into a helper returning sibling session
  IDs plus a read-failure signal, and keep its existing boolean caller on it.
- Call the helper from `Executor.LaunchPreparedSession` when it starts the
  agent, and from `Executor.resumeSession`, before the process starts.
- Add `session_coresidency_admitted_total` (labelled by `site`) and
  `session_coresidency_observation_skipped_total` (labelled by `reason`).
- Add the park interaction warning to `docs/public/tasks-and-workflows.md`
  where it currently says a parked session is not an active process.
- Add Go unit coverage for the co-resident, solitary, and read-failure cases.

## Out of scope

- Refusing, deferring, or serializing any launch or resume.
- Changing the default `profile_session_end_policy`.
- Frontend, board indicators, or session-tab surfaces.
- Changing `is_primary` semantics.
- Instance-wide session-ceiling behavior.

## Acceptance

- A launch or resume with a working sibling increments the counter once for
  that start and logs the sibling session IDs.
- A launch or resume with no working sibling records nothing.
- A failing sibling read records a skip with its reason, and the launch or
  resume still proceeds.
- Counter label keys are exactly `site` and `reason`; no identifier appears as
  a label value.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/executor
cd apps/backend && make lint
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
```

Each new unit test must fail before the production change and pass after it.

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/session_coresidency_metrics.go`
- `apps/backend/internal/orchestrator/executor/session_coresidency_test.go`
- `docs/public/tasks-and-workflows.md`
- `docs/plans/concurrent-session-worktree-visibility/plan.md`
- `docs/plans/concurrent-session-worktree-visibility/task-01-observe-concurrent-worktree-sessions.md`

## Dependencies

None.

## Risks

- Placing the observation after the process starts would report the condition
  too late to be useful in a log read backwards from a corruption.
- Adding identifiers as counter labels would make cardinality unbounded.
- Logging at error level would imply Kandev failed to prevent something it
  deliberately permits; warning with explicit wording is required.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004` and its criteria.
- `docs/specs/tasks/system-design/additional-session-workspace-reuse.md`,
  section "Concurrent session visibility".
- `docs/decisions/2026-08-31-workflow-profile-session-switch-policy.md`.
- `hasOtherWorkingSessions`, `isRuntimeWorkingSessionState`,
  `LaunchPreparedSession`, `resumeSession`, `validateAndLockResume`.
- `apps/backend/internal/orchestrator/office_stall_metrics.go` as the expvar
  label-model precedent.

## Results

Pending implementation.
