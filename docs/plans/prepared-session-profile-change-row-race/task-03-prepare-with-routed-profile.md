---
id: "03-prepare-with-routed-profile"
title: "Prepare Office sessions with the routed execution profile"
status: done
wave: 3
depends_on:
  - "02-keep-session-on-profile-replacement"
plan: "plan.md"
requirements:
  - REQ-OFFICE-SESSION-IDENTITY-001
  - REQ-OFFICE-ROUTING-001
acceptance_criteria:
  - AC-OFFICE-SESSION-IDENTITY-001.9
  - AC-OFFICE-ROUTING-001.2
  - AC-OFFICE-ROUTING-001.3
system_design:
  - ../../specs/office/system-design/task-session-identity-01.md
  - ../../specs/office/system-design/routing-02.md
---

# Task 03: Prepare Office Sessions With the Routed Execution Profile

## Summary

When the task workbench prepares an Office task's session, bind it to the
execution profile Office would route to, so the run that later launches into it
usually finds a matching profile and reuses the prepared runtime instead of
replacing it.

## Root cause

`Service.EnsureSession` (`apps/backend/internal/orchestrator/session_ensure.go`)
resolves the profile with `resolveTaskAgentProfile`, which returns the Office
task's assignee agent ID. That ID flows through `createStartSession` into
`createOfficeSession` as the session's `ExecutionProfileID`. The scheduler's
routed launch uses the concrete profile from `routing.Resolver.Resolve`, so the
IDs differ whenever routing yields a candidate, and every such launch tears down
the prepared runtime.

Two further paths kept the runtime on the agent ID after the first change:

- The task workbench's `session.launch` auto-start is downgraded to a prepare
  while the task is blocked and reaches `PrepareTaskSession` without going
  through `EnsureSession`, so the preview has to live in `PrepareTaskSession`.
- `PrepareTaskSession`'s background workspace launch adopts the dynamic
  profile resolver's result. For a non-dynamic profile that resolver echoes the
  profile it was given, and for an Office session it is given the owner's agent
  ID, so it overwrote the routed profile and the `executors_running` row
  recorded the agent ID.

## In scope

- An orchestrator hook (for example `SetOfficeLaunchProfilePreview`) that
  returns the execution profile Office would launch an agent with. It is wired
  in `internal/backendapp` to `routing.Resolver.Resolve` with an empty run
  reason, taking the first candidate's `ExecutionProfileID`. With no candidate,
  or when routing is unavailable, it returns the agent ID (the scheduler's
  legacy path).
- `PrepareTaskSession` uses the hook for Office tasks whose resolved profile is
  the assignee agent, keeping the agent ID as the session owner. This covers
  both `session.ensure` and the downgraded `session.launch` prepare.
- The prepare's background launch adopts the dynamic resolver's result only
  when it chose a concrete profile different from the logical one.
- Preparation records no route attempt, changes no provider health, and never
  parks a run.

## Out of scope

- Re-resolving an already-prepared session when routing changes; Task 02's
  replacement path owns that.
- Reason-specific tiers (`tier_per_reason`): preparation has no run reason, so
  a reason-tier launch can still differ and uses the replacement path.
- Non-Office tasks.

## Acceptance

- With routing resolving a candidate, opening the task workbench prepares the
  session with that candidate's execution profile, and the blocker-resolved run
  launches without an `execution profile changed` stop.
- With routing changed between preparation and launch, the Task 02 behavior
  holds: replacement stop, session live, run launched.
- Preparation leaves `office_run_route_attempts` and provider health unchanged.

## Verification

Write the tests first and confirm each fails before the production change.

```bash
# From apps/backend:
go test -trimpath ./internal/orchestrator -run 'TestEnsureSession_OfficeTask|TestPrepareTaskSession_|TestLaunchPreparedSession_ProfileReplacement' -count=1 -race
go test -trimpath ./internal/backendapp -run TestOfficeLaunchProfilePreview -count=1
# From repo root:
bash scripts/lint-go-changed <changed-go-files>
# From apps/web:
pnpm e2e:run --host --shards 1 --project chromium tests/office/prepared-session-profile-replacement.spec.ts
```

The E2E spec gains a second case: the routed-profile match (no replacement stop
in the run's route attempts and the prepared execution reused). The existing
case changes routing after preparation so it keeps exercising the replacement
path.

## Files likely touched

- `apps/backend/internal/orchestrator/session_ensure.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/service.go` (hook field)
- `apps/backend/internal/backendapp/main.go` (wiring next to the scheduler's resolver)
- `apps/backend/internal/backendapp/office_launch_profile_preview.go` and its test (new)
- `apps/backend/internal/orchestrator/session_ensure_office_profile_test.go` (new)
- `apps/web/e2e/tests/office/prepared-session-profile-replacement.spec.ts`

## Dependencies

- Task 02 (replacement path for profile drift).

## Risks

- A second copy of the resolution logic would drift from the scheduler. Call
  the same `Resolver.Resolve`, never reimplement tier or provider selection.
- `Resolve` reads provider health; a degraded provider can make preparation
  pick a fallback. That matches what dispatch would pick at that moment.

## Parallelism

`sequential`

## Inputs

- `routing-02.md` Session/executor binding guarantee (amended for this task).
- Dev-instance and E2E evidence: prepared session bound to the agent ID, run
  launched with the routed profile.

## Results

- `PrepareTaskSession` prepares an assigned Office task's session with the
  profile returned by `SetOfficeLaunchProfilePreview`. Without a candidate, or
  when the preview fails (logged as a warning), it keeps the agent ID.
- The prepare's background launch no longer replaces that profile with the
  dynamic resolver's echo of the owner ID; it adopts the resolver's result only
  for a concrete dynamic choice.
- `backendapp` wires the preview to the scheduler's own `routing.Resolver`
  with empty `ResolveOptions` and returns the first candidate's profile.
- Tests written first and confirmed red:
  - `TestEnsureSession_OfficeTaskUsesRoutedProfile`: session bound to
    `agent-ceo` instead of `profile-routed` before the first change.
  - `TestPrepareTaskSession_OfficeTaskUsesRoutedProfile`: with the production
    profile resolver installed, the session row was routed but the runtime
    launch used `agent-ceo`.
  - `TestOfficeLaunchProfilePreview_*`: stub returned `""` and no error.
  - E2E "prepared session binds to the routed profile the run launches with"
    now also asserts the prepared execution is reused (same
    `agent_execution_id`). Against the earlier commit it failed because the
    runtime was replaced; manual testing on a dev instance showed the same
    `execution profile changed` stop. The second E2E case opens the task before
    routing is enabled and asserts the Task 02 replacement path.
- Verified with:
  - `go test -trimpath ./internal/orchestrator -run 'Dynamic|Prepare|EnsureSession|LaunchPrepared' -count=1`
  - `go test -trimpath ./internal/backendapp -run TestOfficeLaunchProfilePreview -count=1`
  - `pnpm e2e:run --host --shards 1 --project chromium --repeat-each 3 tests/office/prepared-session-profile-replacement.spec.ts` (6/6)
  - `make fmt`, `make typecheck`, `make lint` (Node heap raised for `tsc`)
