---
created: 2026-09-18
status: implemented
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
system_design:
  - ../../specs/tasks/system-design/attach-workspace-sources.md
legacy_specs: []
---

# Implementation Plan: Materialize Legacy Branches in Inherited Workspaces

## Overview

Repair `add_branch_to_task_kandev` for live `inherit_parent` and `shared_group`
tasks. Resolve the environment used by the selected child session, validate its
owner and executor, materialize the sibling worktree into that environment's
canonical inventory, and preserve atomic rollback on every failure.

The change is one backend work order. It starts with failing service and
materializer regressions, implements the narrow environment-resolution repair,
and runs the complete affected package tests.

## Confirmed root cause

- `Service.taskAlreadyLaunched` and
  `requireWorktreeExecutorForBranchAdd` inspect only
  `GetTaskEnvironmentByTaskID(childTaskID)`.
- `branchMaterializer.prepareMaterializeRequest` selects the active child
  session, then repeats the task-owned environment lookup instead of following
  `session.TaskEnvironmentID`.
- An inherited child has no child-owned environment row. Its session points to
  the parent-owned or group-owned canonical environment, so both checks
  misclassify the live task as pre-launch.
- The materializer logs `skipping materialize: task environment not provisioned
  yet` and returns a deferred result. The service accepts that result and keeps
  the new `task_repositories` row.
- Launch and resume correctly fall back to the session-bound environment and
  validate its canonical `task_environment_repos` inventory. The retained
  attachment has no matching physical row, so recovery fails with
  `ErrWorkspaceReuseUnsafe` before agent startup.

A temporary SQLite-backed service regression reproduced the accepted deferred
result and retained attachment on current `main`. A second temporary executor
test confirmed that the next resume rejects the incomplete inherited inventory.
Both diagnostic tests were removed after capture; the work order replaces them
with permanent TDD coverage.

## Scope

### In scope

- Resolve the effective add-branch environment from the selected eligible
  session's `TaskEnvironmentID`, falling back to the task-owned row only when
  no eligible session is bound.
- Validate that a foreign environment has a present, unarchived owner and uses
  the worktree executor.
- Make the service's live/pre-launch decision and the materializer's target use
  the same environment and eligible-session rules.
- Persist the new physical worktree row under the inherited environment and
  return the exact sibling and promoted task-root paths.
- Compensate the attachment and any repository entity created by the request
  when resolution, validation, materialization, or inventory persistence fails.
- Preserve intentional deferral for a task that has no effective environment
  and no eligible bound session.

### Out of scope

- Repairing individual stale production records or moving a manually created
  worktree into Kandev inventory.
- Changing inheritance, workspace-group ownership, launch/resume inventory
  validation, branch naming, or executor selection.
- Supporting the legacy worktree-only action on Local, Docker, SSH, Sprites,
  Kubernetes, or other non-worktree executors.
- Schema migrations, frontend changes, UI copy, public documentation, or a new
  recovery fallback based on filesystem discovery.

## Technical approach

### Effective environment resolution

Replace the task-owned-row boolean with a resolver that produces an explicit
pre-launch or live result. Select the most recently updated eligible task
session using the materializer's existing state set and load its referenced
environment by ID. Fall back to a task-owned environment only when that session
has no environment binding, so an older owned row cannot override the runtime
identity selected by a handoff or shared workspace transition.

For a foreign environment, load its owner task and fail closed when the owner
is missing, unreadable, or archived. Propagate repository errors instead of
converting them into a false pre-launch result. Reject a resolved environment
whose executor is not `worktree` before retaining a new attachment.

Keep session and environment identity together through materialization. Carry
the preflight identity across attachment persistence, compare it with a fresh
resolution, and reject disappearance or replacement before calling the
materializer. Before creating the worktree, verify again that the selected
session is still bound to the resolved environment and that the environment
has the provisioned task-root identity required by the worktree manager.

### Canonical materialization and compensation

Pass the selected session to the existing worktree manager. Its store locks
and derives `task_environment_repos.task_environment_id` from
`task_sessions.task_environment_id` in the inventory write transaction, so an
inherited session writes into the parent-owned or group-owned canonical
inventory without a second ownership model or a rebind race.

Keep the current sibling creation, workspace-path promotion, agentctl rescan,
and materialized-worktree event. A live success is complete only when the
result carries both paths and the canonical inventory row exists. All error or
empty-live-result paths return through `commitWorkspaceSourceBatch`
compensation before `task.updated` is published.

## Work orders

- [x] [Task 01: Resolve and materialize the inherited live environment](task-01-materialize-inherited-environment.md)

## Dependency order

```text
Task 01
```

The resolver, materializer, and regressions form one atomic backend boundary,
so the work is sequential.

## Verification strategy

- Run the inherited service regressions red before production changes. They
  prove an inherited live environment cannot be treated as pre-launch and a
  non-worktree inherited environment is rejected without a retained row.
- Run the materializer regression red before its resolver change. It proves
  the selected child's environment binding creates the sibling and canonical
  environment-repository row under the foreign owner.
- Run the focused tests green, then the complete task-service and backendapp
  package tests.
- Run specification validation and diff checks after recording results.

No browser test is required. The repair changes backend environment resolution
and persistence only; existing desktop and mobile projections consume the same
materialized-worktree event and canonical inventory.

## Risks

- Divergent session-selection rules can let service preflight and materializer
  target different environments. Keep one ordering/state contract and carry or
  revalidate the exact identities at the mutation boundary.
- Treating an environment lookup failure as absence recreates the original
  stale-row defect. Only a positive lack of both environment and eligible bound
  session permits deferral.
- Mutating a foreign environment without owner validation can write into an
  archived or orphaned workspace. Match the launch/resume owner checks.
- A worktree can be created before inventory persistence fails. Preserve the
  worktree manager's compensation and the attachment batch rollback rather
  than adding filesystem guessing or repair.

## Package handoff

Implement the single work order with TDD. Record the red and green commands in
the work order, update both statuses after verification, and keep the current
requirement and ADR identities. No new ADR is needed because the repair
enforces the accepted live add-branch and inherited-workspace boundaries.
