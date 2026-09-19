---
id: "01-gate-focus-autostart"
title: "Gate focus-induced auto-start"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-DEPENDENCIES-001
acceptance_criteria:
  - AC-TASKS-TASK-DEPENDENCIES-001.1
system_design:
  - ../../specs/tasks/system-design/task-dependencies.md
---

# Task 01: Gate Focus-Induced Auto-Start

## Summary

Make `launchStart` apply the task dependency gate to automatic starts before it
can call `startTask`. Blocked and unreadable dependency state downgrade to the
existing workspace-only prepare flow, while an unblocked task still starts.

## In scope

- Add focused `LaunchSession` regression coverage for blocked, unblocked, and
  dependency-read-error cases.
- Extend `launchStart`'s `req.AutoStart` downgrade condition with
  `dependencyBlocksAutoStart(ctx, req.TaskID, "session.launch")`.
- Preserve the request fields needed by `launchPrepare` and its passthrough
  recursion guard.

## Out of scope

- Gating manual starts where `AutoStart` is false.
- Changing `autoStartTaskForStep`, dependency resolution, WIP admission, or the
  frontend `session.ensure` message.
- Adding a browser test for a backend launch-admission regression.

## Acceptance

- A blocked automatic `IntentStart` creates only a `CREATED` workspace session,
  never launches the agent, and never moves the task to `SCHEDULING` or
  `IN_PROGRESS`.
- A dependency-read error has the same fail-closed prepare-only result.
- An unblocked automatic `IntentStart` follows the existing normal start path.

## TDD sequence

1. Add `TestLaunchSession_AutoStartDependencyGate` with a nearby
   `@covers AC-TASKS-TASK-DEPENDENCIES-001.1` annotation and three cases:
   unresolved, resolved, and read error.
2. Run only that test. Confirm the unresolved and read-error cases fail because
   an agent starts or the task leaves `CREATED`; confirm the resolved positive
   control reaches the existing launch path.
3. Add the minimum dependency predicate to `launchStart`'s current automatic
   downgrade condition.
4. Rerun the focused test and keep all three cases green. Refactor only the test
   setup if needed for clarity, then rerun it after the final production edit.

## Verification

```bash
cd apps/backend
go test -tags fts5 ./internal/orchestrator -run '^TestLaunchSession_AutoStartDependencyGate$' -count=1
cd ../..
make -C apps/backend test
make -C apps/backend lint
gofmt -l apps/backend/internal/orchestrator/session_launch.go apps/backend/internal/orchestrator/session_launch_dependency_test.go
git diff --cached --name-only | grep '\.go$' | xargs -r gofmt -l
```

If either formatting command reports a file, run `gofmt -w` on that exact file,
restage it, and repeat both checks before committing.

## Files likely touched

- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/session_launch_dependency_test.go`

## Dependencies

None.

## Risks

- A test that observes only a session row could miss an agent launch; assert
  both persisted state and the launch counter.
- The resolved positive control must use the same harness so a no-op setup
  cannot make every case appear gated.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-TASK-DEPENDENCIES-001` and
  `AC-TASKS-TASK-DEPENDENCIES-001.1`.
- `docs/specs/tasks/system-design/task-dependencies.md`, especially automated
  launch guards and fail-closed behavior.
- `apps/backend/internal/orchestrator/session_launch.go` and existing
  orchestrator dependency/launch test helpers.

## Results

Implemented the dependency gate in `launchStart` and added
`TestLaunchSession_AutoStartDependencyGate`. The regression covers unresolved
dependencies, resolved dependencies, and dependency-read errors at the
`LaunchSession` boundary. Blocked and unreadable dependency state now keeps the
request on the workspace-only `CREATED` path; resolved state still launches.

Validation:

- Focused test passed.
- Focused race test passed.
- Backend lint passed with zero issues.
- `gofmt -l` reported no changes.
- The full backend target reached and passed `internal/orchestrator`, but the
  target returned failure because unrelated config-discovery tests in
  `internal/common/config` and `internal/launcher` read the existing
  `/root/.kandev/config.yaml` instead of their temporary homes.
