---
id: "01-push-base-branches-every-workspace"
title: "Push stored base branches to every workspace"
status: done
wave: 1
parallelism: sequential
depends_on: []
plan: "plan.md"
spec: "../../specs/workspaces/requirements/workspace-base-branch-propagation.md"
---

# Task 01: Push stored base branches to every workspace

## Acceptance

- The stored per-repo base-branch map reaches agentctl's `WorkspaceTracker` for
  workspaces created outside the full `LaunchAgent` path — specifically
  `startAgentOnExistingWorkspace` and workspace-only / lazy-recovery execution
  creation after a backend restart.
- The map is hydrated with `Service.collectTaskBaseBranches`, so the repository
  key derivation (including the single-repo empty-key fallback) is shared with
  the edit-time path rather than re-derived.
- The push happens at one chokepoint covering every creation path, not as a
  separate call bolted onto each site.
- The push is best-effort: failure is logged at warn, never fails workspace
  creation or agent start.
- Re-pushing an unchanged map is a no-op (`SetBaseBranches` is already
  idempotent).
- The full-launch path keeps working unchanged — `collectBaseBranches` in
  `manager_launch.go` is not removed or bypassed.

## Regression test

Pin the **propagation**, not the git arithmetic — asserting on diff numbers
would require a fixture repo and would not fail for the right reason.

- **Red first.** Drive the existing-workspace / recovery execution path with a
  task whose `task_repositories.base_branch` is set, and assert that the
  agentctl client received a `SetBaseBranches` call carrying that map. Before
  the fix, zero calls are made.
- Cover the multi-repo key shape and the single-repo empty-key fallback, since
  a wrong key silently reproduces the bug for multi-repo tasks.
- Assert the push is not fatal when the client returns an error.
- Join the existing patterns in
  `internal/agent/runtime/lifecycle/base_branches_metadata_test.go`; do not
  start a third harness.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle/... ./internal/task/service/... -race)
```

PASS — `ok .../internal/task/service 40.100s`, `ok .../internal/agent/runtime/lifecycle 22.957s`,
`ok .../internal/agent/runtime/lifecycle/skill 3.889s`.

```bash
(cd apps/backend && go test ./internal/agentctl/server/process/... ./internal/orchestrator/... -race)
```

PASS — `ok .../internal/agentctl/server/process 90.732s`; every `./internal/orchestrator/...`
package reported `ok` or had no test files.

## Files Likely Touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_base_branches.go`
- `apps/backend/internal/task/service/service_branch_update.go`
- `apps/backend/internal/agent/runtime/lifecycle/base_branches_metadata_test.go`
- one of `internal/agent/runtime/lifecycle/` (attach chokepoint) or
  `internal/orchestrator/executor/executor_execute.go`, depending on where the
  chokepoint lands
- `apps/backend/internal/backendapp/main.go` if a provider callback is wired

## Dependencies

None.

## Inputs

- Spec scenarios 1, 2, 4 and the multi-repo failure mode.
- `collectBaseBranches` (`manager_launch.go:173`) — the launch-time shape to
  mirror.
- `Service.collectTaskBaseBranches` — the DB-time hydrator to reuse.
- `Manager.PushBaseBranchesForTask` and `Client.SetBaseBranches` — the existing
  push, already idempotent and best-effort.
- Existing wiring at `internal/backendapp/main.go:439`.

## Output Contract

The provider is wired before lifecycle startup, so recovered executions and
agentctl-ready executions are seeded from the task's recorded base-branch map.
For workspaces created outside the full launch path, agentctl may publish one
fallback-based status update before the readiness-time push lands; the push
then refreshes the tracker and subsequent stats use the recorded base. No
signature change to `PushBaseBranchesForTask` or `SetBaseBranches`.
