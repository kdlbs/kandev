---
spec: docs/specs/workspaces/requirements/workspace-base-branch-propagation.md
created: 2026-08-05
status: complete
---

# Implementation Plan: Workspace Base-Branch Propagation

## Overview

Two backend changes. Task 01 makes the stored base-branch map reach agentctl on
every workspace, not only full launches, by pushing it from a single chokepoint
once the agentctl client is available. Task 02 makes the integration-branch
fallback observable so this class of defect cannot recur silently.

No schema, API, or persistence change. Backend only; no frontend work — the card
renders whatever stat the backend reports.

## Confirmed root cause

`MetadataKeyBaseBranches` is produced in exactly one place —
`collectBaseBranches` at
`internal/agent/runtime/lifecycle/manager_launch.go:173`, on the full
`LaunchAgent` path. The only other producer is
`Service.UpdateRepositoryBaseBranch` →
`Manager.PushBaseBranchesForTask`, which fires only when a user edits the base
branch.

`startAgentOnExistingWorkspace` (`internal/orchestrator/executor/executor_execute.go`)
and the workspace-only / lazy-recovery execution creation that runs after a
backend restart both bypass `LaunchAgent`. Their agentctl instances therefore
receive no base-branch map, so `WorkspaceTracker.BaseBranch()` is empty,
`resolveBaseBranch` skips `resolveStoredRef`, and the tracker falls through to
`branchDiffCandidates` (`origin/main`, `origin/master`, `main`, `master`).
`computeBaseCommit` then returns the merge-base against whichever candidate
resolves first.

Reproduced exactly against a repository whose integration line is neither `main`
nor `master`:

| Quantity | Value |
|---|---|
| Configured base / true merge-base | the task's recorded feature base → correct SHA |
| True diff | `+66 −1` |
| `git merge-base HEAD origin/master` | an ancient commit |
| `git diff --shortstat <that>` | `2156 files changed, 440466 insertions, 28354 deletions` |
| `+` untracked additions (2 files, 7 + 72 lines) | **+440545** |
| Displayed on the card | **+440545 −28354** |

A sibling task in the same repository whose workspace *was* created through the
full launch path displayed correctly (`+542 −32`), and a second affected task
shared the identical `−28354` because it resolved to the same fallback base.

## Backend

### Task 01 — push the stored base branches on every workspace

The pieces already exist and are wired:

- `Service.collectTaskBaseBranches(ctx, taskID)`
  (`internal/task/service/service_branch_update.go`) hydrates the per-repo map
  from `task_repositories` + `repositories`, including the single-repo empty-key
  fallback. It is the DB-time mirror of `collectBaseBranches`.
- `Manager.PushBaseBranchesForTask(ctx, taskID, branches)`
  (`internal/agent/runtime/lifecycle/manager_base_branches.go`) pushes to every
  running execution of the task and is already idempotent.
- `services.Task.SetAgentBaseBranchPusher(lifecycleMgr)`
  (`internal/backendapp/main.go:439`) already connects them.

What is missing is a caller on the non-launch paths. Add a service entry point
that pairs hydrate + push, and invoke it from the chokepoint where an agentctl
client becomes available for an execution, so every creation path is covered by
one call rather than each path remembering.

Prefer the single chokepoint in the lifecycle manager (where the agentctl client
is attached / the workspace stream connects) over sprinkling calls into
`startAgentOnExistingWorkspace` and each recovery site. If the lifecycle manager
cannot reach the task service directly, follow the existing callback style in
this codebase (`SetOnAgentStartFailed`, `SetMCPIdentityScoper`,
`SetAgentBaseBranchPusher`) and inject a base-branch **provider** the manager
calls at attach time.

Keep the push best-effort: a failure is logged at warn and never fails the
workspace, matching `PushBaseBranchesForTask`'s existing contract.

### Task 02 — make the fallback observable

`resolveBaseBranch` currently falls through silently. Log the fallback once per
resolution with the repository name and the candidate chosen, so a wrong base
shows up in the logs instead of only as a suspicious number on a card.

Keep it cheap — this runs on a status poll. A single log line at the point of
fallback is sufficient; do not add a DB read to determine whether a base branch
"should" have been present.

## Frontend

None. The card renders the backend-reported stat.

## Waves

| Wave | Task | Parallel-safe |
|---|---|---|
| 1 | 01 — push stored base branches on every workspace | no |
| 2 | 02 — log integration-branch fallback | no |

`parallelism: sequential` on both. Task 02 is independent in principle but
touches the same resolution path Task 01's test exercises, so keep it ordered.

## Validation

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle/... ./internal/agentctl/server/process/... ./internal/task/service/... -race)
```

```bash
(cd apps/backend && golangci-lint run ./... --new-from-rev=origin/main --timeout=5m)
```

## Risks

- `PushBaseBranchesForTask` iterates every execution of the task. Calling it
  from a per-execution attach point means O(executions) pushes per attach. For
  the common one-or-two-execution task this is negligible, but prefer a
  per-execution push if the chokepoint makes that natural.
- The tracker's map is keyed by repository name matching the worktree directory
  basename. `collectTaskBaseBranches` already encodes that mapping including the
  single-repo empty-key fallback — reuse it rather than re-deriving the key, or
  multi-repo tasks will silently miss.
- Existing coverage lives in
  `internal/agentctl/server/process/workspace_git_status_base_branch_test.go`
  and `internal/agent/runtime/lifecycle/base_branches_metadata_test.go`; the new
  regression test should join those rather than starting a third pattern.

## Out of scope

Carried from the spec: the integration candidate list stays as-is, untracked-file
folding is unchanged, `correctStaleComparisonBase` is unchanged, and no
backfill/recompute of existing workspaces (the value is not persisted and
refreshes on the next poll).
