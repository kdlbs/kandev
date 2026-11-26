---
id: "01-preserve-workspace-launch"
title: "Keep launch active during workspace reads"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004
acceptance_criteria:
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.12
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.1
system_design:
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 01. Keep launch active during workspace reads

## Summary

Keep the materialization owner's launch active when a concurrent workspace
read reports a `creating` environment. The same session operation still
controls runtime creation.

## In scope

- Identify the refusal for a `creating` environment at the workspace
  admission boundary.
- Make `coalesceSessionLaunch` use its normal launch admission after it
  receives that result from a workspace operation.
- Add the deterministic backend regression. Run the existing PR-worktree
  browser test.

## Out of scope

- Workspace replacement and recovery policy.
- GitHub branch selection.
- Frontend changes.

## Acceptance

- The workspace caller receives `ErrSessionWorkspaceNotReady`. The concurrent
  launch registers one runtime through its normal path.
- Creating-environment refusals do not consume the recovery admission retry
  budget.
- If the originating launch returns an error, the caller receives that
  error and the orchestrator settles its materialization claim.
- Cancellation ends the affected caller's wait. Concurrent callers can
  share the result of an admitted launch within their own deadlines.

## Regression setup

1. In `TestFreshLaunchContinuesAfterWorkspaceReadNotReady`, bind a
   `CREATED` session to its own `creating` Worktree environment.
2. Hold the workspace-info read inside `GetOrEnsureExecution` with a channel.
3. Release the read after `Manager.Launch` joins the same operation.
   Reuse the wait-context pattern in `recovery_launch_coalescing_test.go`.
4. Check the reader's error and the launch's registered execution. Count
   runtime registrations.
5. Repeat the handoff with three successive reader-owned refusals before
   launch admission, then confirm launch registers one runtime.

The baseline passes the launch-only control. The shared result fails the
concurrent case with the incident error. Use a genuine launch error and a
cancelled waiter as controls for result handling. In the concurrent success
case, use two launch waiters and check that they receive the same execution.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle \
  -run '^TestFreshLaunchContinuesAfterWorkspaceReadNotReady$' -count=20)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle \
  -run '^(TestRecoveryLaunchDoesNotInheritConcurrentWorkspaceAdmissionFailure|TestLaunchDeadlineDoesNotAbortLiveEnsureFollower|TestEnsureLeaderDoesNotHideLaunchWaiterDeadline|TestGetOrEnsureExecution_DedupAcrossEnvAndSessionPaths|TestWorkspaceLookupWaitsForCleanupThenUsesItsOwnResult)$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator/executor \
  -run '^TestLaunchPreparedSessionReleasesOwnedClaimOnEarlyError$' -count=1)
```

If the workspace dependencies are absent, install them before the browser test.

```bash
(cd apps && pnpm install --frozen-lockfile)
```

```bash
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium \
  tests/task/create-task-github-url.spec.ts -- --grep \
  'creates task from PR URL with worktree executor and verifies branch'
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_admission.go`
- `apps/backend/internal/agent/runtime/lifecycle/recovery_launch_coalescing.go`
- `apps/backend/internal/agent/runtime/lifecycle/recovery_launch_coalescing_test.go`

## Dependencies

None.

## Risks

Keep the same key for launches and workspace reads. Both callers can otherwise
start competing runtimes. Keep the caller context on every attempt to join
the shared operation.

## Parallelism

`sequential`

## Inputs

- [Requirement AC-004.12](../../specs/tasks/requirements/task-launch-failure-recovery.md#req-tasks-task-launch-failure-recovery-004-abandoned-workspace-materialization-recovery)
- [Workspace requests during initial launch](../../specs/tasks/system-design/task-launch-failure-recovery.md#workspace-requests-during-initial-launch)

## Results

| Lifecycle scenario | Proof | Result |
| --- | --- | --- |
| Concurrent operations and duplicate runtime prevention | `TestFreshLaunchContinuesAfterWorkspaceReadNotReady` and the five lifecycle compatibility tests. | Passed. The regression passed 20 race-enabled runs, including three successive reader-owned refusals before launch admission; launch registered one runtime and two live launch waiters shared it. |
| Cancellation and cleanup ownership | The regression's canceled waiter plus `TestLaunchDeadlineDoesNotAbortLiveEnsureFollower` and `TestEnsureLeaderDoesNotHideLaunchWaiterDeadline`. | Passed. Cancellation ended the affected caller's wait while live callers completed. |
| Materialization claim on launch error | `TestLaunchPreparedSessionReleasesOwnedClaimOnEarlyError` and the regression's genuine launch-error control. | Passed. The launch error remained visible and the existing claim-release test passed. |
| Interactive start from a PR URL | Chromium test “creates task from PR URL with worktree executor and verifies branch.” | Passed: 1 test, 1 worker. |
| Build and changed-file lint | `make -C apps/backend build`; `bash scripts/lint-go-changed` for the four changed lifecycle Go files. | Passed. |

The repair operates during launch admission. Restart recovery and delivery
of accepted prompts use their existing durable identities.
