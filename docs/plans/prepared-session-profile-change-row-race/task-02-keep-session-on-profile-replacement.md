---
id: "02-keep-session-on-profile-replacement"
title: "Keep the session alive across a profile-replacement stop"
status: done
wave: 2
depends_on:
  - "01-tolerate-cleared-executor-row"
plan: "plan.md"
requirements:
  - REQ-OFFICE-SESSION-IDENTITY-001
  - REQ-OFFICE-ROUTING-001
acceptance_criteria:
  - AC-OFFICE-SESSION-IDENTITY-001.9
  - AC-OFFICE-ROUTING-001.3
system_design:
  - ../../specs/office/system-design/task-session-identity-01.md
  - ../../specs/office/system-design/routing-02.md
---

# Task 02: Keep the Session Alive Across a Profile-Replacement Stop

## Summary

When `LaunchPreparedSession` stops a prepared execution bound to another
profile, the resulting `agent.stopped` handling must not move the session to
`CANCELLED`. The launch then registers its new execution on the same session.

## In scope

- Failing orchestrator integration test
  `TestLaunchPreparedSession_ProfileReplacementKeepsSessionLaunchable`:
  `NewService` on a real SQLite repository; a prepared session with an
  `executors_running` row whose `ExecutionProfileID` differs from the launch
  profile; an agent manager whose `StopAgentWithReason` deletes the row and
  calls `Service.handleAgentStopped` synchronously, as the lifecycle manager
  does. Assert the launch succeeds, the session is not `CANCELLED`, and the new
  execution is launched.
- Failing Office E2E spec
  `apps/web/e2e/tests/office/prepared-session-profile-replacement.spec.ts`:
  a mock `claude-acp` provider with balanced-tier routing so the run resolves a
  routed execution profile, a task assigned to the seeded agent with a pending
  blocker, the task workbench (`/t/:id`) opened so the UI prepares its session,
  then the blocker moved to the final step. Assert the `task_blockers_resolved` run launches and is not parked, and
  the task's session is not `CANCELLED`.
- In the profile-change branch of `LaunchPreparedSession`, call
  `registerStopOwner(sessionID, running.AgentExecutionID, true)` before
  `StopAgentWithReason`.

## Out of scope

- Changing `handleAgentStopped` or the teardown-claim registry.
- Other callers of `StopAgentWithReason`.

## Acceptance

- Both new tests fail before the production change: the integration test with
  the session `CANCELLED` / `session is terminal`, the E2E spec with a parked or
  failed run.
- Both pass after it, together with Task 01's tests.

## Verification

```bash
# From apps/backend:
go test -trimpath ./internal/orchestrator -run 'TestLaunchPreparedSession_ProfileReplacement' -count=1 -race
go test -trimpath ./internal/orchestrator/executor -count=1
# From repo root:
bash scripts/lint-go-changed <changed-go-files>
# From apps/web:
pnpm e2e:run --host --shards 1 --project chromium tests/office/prepared-session-profile-replacement.spec.ts
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/prepared_session_profile_replacement_test.go` (new)
- `apps/web/e2e/tests/office/prepared-session-profile-replacement.spec.ts` (new)

## Dependencies

- Task 01 (row-deletion tolerance).

## Risks

- `RegisterExecutionStopOwner` returns without registering when the session's
  cancel guard is contended or a cancellation is in flight. In that case the
  existing cancellation owns the session, and failing the launch is correct.

## Parallelism

`sequential`

## Inputs

- Dev-instance log of the failed launch after Task 01 (plan root cause).
- `handleAgentStopped` teardown-owner acknowledgement path.

## Results

- `LaunchPreparedSession` registers explicit stop ownership for the prepared
  execution before the profile-replacement stop, so `handleAgentStopped`
  treats the stop as an acknowledgement and leaves the session launchable.
- `TestLaunchPreparedSession_ProfileReplacementKeepsSessionLaunchable` failed
  before the change with `session state superseded by terminal transition:
  session session-replace is CANCELLED`, and passes after it.
- The E2E spec failed before the change with the prepared session `CANCELLED`
  and passes after it (3/3 with `--repeat-each 3`). Two setup findings: the
  Office task page (`/office/tasks/:id`) does not prepare a session, and the
  default E2E seed's mock agent is not routable, so the launch reuses the
  session's own profile unless a routable mock provider is configured in the
  test body (the `testPage` fixture resets workspace routing).
- Verified with:
  - `go test -trimpath ./internal/orchestrator -run 'TestLaunchPreparedSession_ProfileReplacement' -count=1 -race`
  - `go test -trimpath ./internal/orchestrator/executor -count=1`
  - `pnpm e2e:run --host --shards 1 --project chromium --repeat-each 3 tests/office/prepared-session-profile-replacement.spec.ts`
  - `make fmt`, `make typecheck` (with `NODE_OPTIONS=--max-old-space-size=8192`), `make lint`
