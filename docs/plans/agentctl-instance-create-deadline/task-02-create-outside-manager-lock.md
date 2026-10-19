---
id: "02-create-outside-manager-lock"
title: "Create instances without holding the manager lock across startup"
status: done
wave: 2
depends_on:
  - "01-instance-create-deadline"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002
acceptance_criteria:
  - AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002.1
  - AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002.2
  - AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002.3
  - AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002.4
  - AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002.5
system_design:
  - ../../specs/executors/system-design/agentctl-instance-create-deadline.md
---

# Task 02: Create instances without holding the manager lock across startup

## Summary

`instance.Manager.CreateInstance` holds `mu` only for admission and
publication. Concurrent creations overlap their startup, and operations on
other instances no longer wait for it.

## In scope

- Split `CreateInstance` into admission, startup, and publication.
- Start the HTTP server during publication, under `mu`.
- Add `activeCreations` and wait on it at the start of `Shutdown`'s drain.
- Store the process manager on the provisional bundle under `cleanupMu`.
- Update the comments that describe creation as serialized or bounded by a
  30-second client.
- Add the manager tests listed in the [plan](plan.md#tests).

## Out of scope

- Changes to `ControlClient`, the control API, lifecycle callers, or executors.
- Changes to abandonment cleanup, port allocation, `StopInstance` teardown, or
  the idle reaper beyond their lock scope.
- A concurrency limit, retry, or classification for creation.

## Acceptance

1. Before the change, `TestConcurrentCreationsOverlapStartup` fails because
   only one creation reaches the startup barrier. After the change, all four
   reach it and succeed.
2. A duplicate ID is rejected during startup, other instances stay reachable
   during startup, and shutdown waits for an in-progress creation without
   leaving a published instance or reserved port.
3. The existing abandonment, shutdown, port, and turn-outcome wiring tests in
   the package pass, together with the package build and vet.

## TDD sequence

1. Mark this work order `in_progress` and synchronize the plan.
2. Add `TestConcurrentCreationsOverlapStartup`, run it on the unchanged
   manager, and record the failure.
3. Add the duplicate-ID, other-instance, and shutdown tests. Record which fail
   on the unchanged manager.
4. Split the creation phases, start the server during publication, and add
   `activeCreations`.
5. Run all verification commands and record their results.
6. Mark this work order `done` only after its checks pass.

Hold creations with the existing `afterTrackerStart` seam. Bound every wait in
the tests, release held creations before failing, and shut down each manager in
cleanup. Use a probed bindable loopback port range and `t.TempDir()` workspaces.

## Verification

Run from the repository root. The first command is also the required red check
before implementation.

```bash
(cd apps/backend && go test ./internal/agentctl/server/instance -run '^TestConcurrentCreationsOverlapStartup$' -count=1)
(cd apps/backend && go test ./internal/agentctl/server/instance -run 'Test(ConcurrentCreations|CreateInstance|Abandon|Shutdown|OtherInstanceOperations|StopInstance)' -count=1)
(cd apps/backend && go test ./internal/agentctl/server/instance -count=1)
(cd apps/backend && go build ./internal/agentctl/... && go vet ./internal/agentctl/server/instance)
git diff --check
```

Run the race detector on the new tests when the local toolchain supports it:

```bash
(cd apps/backend && go test -race ./internal/agentctl/server/instance -run 'Test(ConcurrentCreations|CreateInstance|Shutdown|OtherInstanceOperations)' -count=1)
```

Run the documentation preflight from
[task 01](task-01-instance-create-deadline.md#verification) with this work
order and the manager files added to the path lists.

## Files likely touched

- `apps/backend/internal/agentctl/server/instance/manager.go`
- `apps/backend/internal/agentctl/server/instance/manager_concurrency_test.go`
  (new)
- `apps/backend/internal/agentctl/server/instance/manager_abandon_test.go`
  (comment only)
- `docs/plans/agentctl-instance-create-deadline/plan.md`
- `docs/plans/agentctl-instance-create-deadline/task-02-create-outside-manager-lock.md`

Read-only dependencies include `internal/agentctl/server/api/control_server.go`,
`instance/idle_reaper.go`, and the existing `manager_*_test.go` files.

## Dependencies

Task 01. It ships the client half of the same failure and stays a separate
commit.

## Risks

- A creation admitted just before shutdown could publish after the shutdown
  snapshot. `activeCreations` must be added under `mu` after the
  `shuttingDown` check.
- Starting the server outside publication would leave a serving, unpublished
  instance when publication is refused.
- A test that does not release its held creations can hang the package. Release
  them on every failure path.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/agentctl-instance-create-deadline.md)
- [System design](../../specs/executors/system-design/agentctl-instance-create-deadline.md)
- [Evidence and root cause](plan.md#evidence-and-root-cause)
- `apps/backend/internal/agentctl/AGENTS.md` standalone instance port leases.
- `apps/backend/AGENTS.md` testing and goroutine ownership conventions.

## Results

Completed on 2026-10-12. `Manager.CreateInstance` now runs `admitCreate` and
`publishInstance` under `mu`, and `startInstance` without it. The HTTP server
starts inside `publishInstance`. Admission counts the creation on
`activeCreations`, and `Shutdown` waits on it before draining `abandonWG`.
Environment validation stays in admission, before any port is reserved. Every
current `CreateRequest` field is carried into the instance overrides. The
existing abandonment test comment no longer describes creation as serialized.

Red checks on the unchanged manager: `TestConcurrentCreationsOverlapStartup`
failed with `fewer than 4 creations reached the end of startup together`,
`TestCreateInstanceRejectsDuplicateIDDuringStartup` with `duplicate creation
waited for the first creation's startup`, and
`TestOtherInstanceOperationsDoNotWaitForStartup` with `operations on another
instance waited for a creation's startup`.
`TestShutdownWaitsForInProgressCreation` passed before and after, as a
preservation check.

After the change, the four new tests passed. The race-enabled run over the
creation, abandonment, shutdown, and stop tests passed. The package build, the
`cmd/agentctl` build, and vet passed.

Two failures are independent of this change. On Windows hosts,
`TestBuildMcpServerConfigs_DropsMissingStdioCommand` fails because it executes
a POSIX shell script. Under `-race` on Windows,
`TestCreateInstanceDoesNotWaitForComparisonTarget` exceeds its timing bound; it
fails the same way with the unchanged manager and passes without `-race`.

PR review follow-up: `publishInstance` now re-checks the caller's context under
`mu`, after the shutdown check, so a caller that gives up while publication
waits for the lock leaves the instance unpublished and the abandonment path
releases its port. `TestCreateInstanceNotPublishedWhenCancelledBeforePublication`
holds `mu` and cancels the caller after the post-startup check; before the fix
the creation returned no error and published the instance, and after the fix it
returns a wrapped `context.Canceled` with no published instance or reserved
port. The concurrency tests now bound every completion wait and their cleanup
shutdown, and run their creations on the test context.
