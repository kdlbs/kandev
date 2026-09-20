---
id: "05-cover-routed-taskless-launch"
title: "Cover routed (provider fallback) taskless launch"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-TASKLESS-001
acceptance_criteria:
  - AC-OFFICE-TASKLESS-001.3
  - AC-OFFICE-TASKLESS-001.4
system_design:
  - ../../specs/office/system-design/taskless-run-sessions.md
---

# Task 05: Cover routed (provider fallback) taskless launch

## Summary

`AC-OFFICE-TASKLESS-001.3` requires concrete **and** provider-routed profiles to
work for taskless runs. The routed path has no test. `launchAgent`'s taskless
branch calls `tryRoutingDispatch` before the direct launcher, and
`office/scheduler/dispatch_routing.go:launchCandidate` calls the same
`StartRunSession` per candidate and then `SetRunSessionID`, but every existing
taskless test drives the direct branch.

The system design states two rules about this path that nothing currently
asserts: each candidate reserves its own attempt row (three candidates leave
three rows, not one row rewritten three times), and no window exists in which
two candidates of the same run hold live executions.

## In scope

Add coverage for the routed taskless launch:

- A routed profile whose first candidate succeeds: one attempt row, the
  candidate's provider and model recorded on it (`adapter`, `model`,
  `execution_profile_id`), and the Office prompt and skills delivered to the
  launcher unchanged.
- A routed profile that falls back once: two attempt rows with attempt numbers
  1 and 2, the first terminal before the second is launched, and
  `runs.session_id` naming the attempt that actually launched.
- The negative half of `AC-OFFICE-TASKLESS-001.1` on this path too: no `tasks`
  row, no `task_sessions` row.
- Usage attribution for a routed attempt reaches the cost ledger joined to its
  own run session (`AC-OFFICE-TASKLESS-001.4`), not to the first attempt.
- The retry half of `AC-OFFICE-TASKLESS-001.2`: the second attempt's session is
  distinct from the first's and neither reuses the other's ACP session. The fire
  half (two fires, two runs) belongs to Task 01.
- The same-run half of `AC-OFFICE-TASKLESS-001.4`: inject a delayed terminal
  event naming attempt 1 **after** attempt 2 is live on the same run, and assert
  it applies a terminal state to attempt 1's own session while leaving attempt 2
  and the run untouched — no completion, no second ledger row, no cleared
  agent-working state. The existing
  `TestTasklessLateCompletionDoesNotFinishSuccessor` builds two separate runs at
  attempt 1 each, so it does not reach this shape, and fallback produces it
  routinely.

Assert the ordering directly — the predecessor's terminal state before the
successor's reservation — rather than only the final row count. A count-only
assertion passes even if both candidates were live at once.

## Out of scope

- Provider classification, parking and backoff policy, which
  `office/scheduler` already covers for task-bound runs and which this path
  reuses unchanged.
- Any production change to the routing dispatcher, without exception. If the
  routed path cannot be driven without changing production behavior, that is a
  finding against `REQ-OFFICE-TASKLESS-001`, not a licence to edit the code
  under test. The launch-failure stop-error gap is already known and is
  deliberately pending follow-up work — see the system design's scheduling and routing
  section — so encountering it is not a new finding and not this order's work.
- Task 01's cron-to-session chain, which drives the direct branch.

## Validation

```text
cd apps/backend
go test ./internal/office/... -run 'Routed|Fallback|Taskless' -v -count=1
make lint
```

## Reference

- `apps/backend/internal/office/scheduler/dispatch_routing.go` —
  `launchCandidate`, and the `SetRunSessionID` write per candidate
- `apps/backend/internal/office/service/scheduler_integration.go` —
  `launchAgent`'s taskless branch and `tryRoutingDispatch`
- `apps/backend/internal/office/service/taskless_lifecycle_test.go` — the
  direct-branch test style to match
- `apps/backend/internal/office/repository/sqlite/costs.go` — the run-session
  join the usage assertion reads

## Dependencies

None. Independent of Tasks 01 and 02.

## Risks

- **Asserting on the stub instead of the record.** A routing test can pass by
  checking what the launcher was called with. Assert the durable
  `office_run_sessions` rows as well.
