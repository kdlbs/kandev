---
status: draft
system: workspaces
created: 2026-08-05
owners:
  - Kandev
---
# Workspace Base-Branch Propagation Requirements

## Overview

The task card's branch diff stat (`+N −M`) is meant to show what the task's branch changed relative to its configured base branch. For workspaces that were not created through a full agent launch, it instead reports a diff against `origin/master`, producing numbers that can be four orders of magnitude too large — and plausible-looking enough that a user may believe the branch really touched thousands of files.

## Requirements

### REQ-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001: Workspace Base-Branch Propagation

**Intent:** The task card's branch diff stat (`+N −M`) is meant to show what the task's branch changed relative to its configured base branch. For workspaces that were not created through a full agent launch, it instead reports a diff against `origin/master`, producing numbers that can be four orders of magnitude too large — and plausible-looking enough that a user may believe the branch really touched thousands of files.

#### Acceptance criteria

- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.1:** The stored per-repo base branch reaches the `WorkspaceTracker` for **every** workspace, regardless of how the workspace came to exist — full launch, agent start on an already-prepared workspace, or post-restart recovery.
- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.2:** The branch diff stat is computed against the configured base branch whenever one is recorded for the task's repository.
- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.3:** The integration-branch fallback (`origin/main` → `origin/master` → `main` → `master`) applies only when no base branch is recorded, which remains its intended purpose.
- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.4:** When the tracker falls back to an integration candidate, that decision is observable in the logs, naming the repository and the candidate chosen. A silent fallback that yields a wrong-but-plausible number is not acceptable.
- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.5:** Pushing the map is idempotent: re-sending an unchanged map is a no-op, and a workspace that already has the correct base is not disturbed.
- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.6:** **GIVEN** a task whose repository has a recorded base branch, **WHEN** the agent starts on an already-prepared workspace, **THEN** the tracker resolves the diff base from that recorded branch, not from an integration candidate.
- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.7:** **GIVEN** the same task, **WHEN** the backend restarts and the execution is recreated by lazy recovery, **THEN** the recreated workspace still resolves the recorded base branch.
- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.8:** **GIVEN** a task whose repository has no recorded base branch, **WHEN** its workspace is created, **THEN** the integration-branch fallback applies as before and the chosen candidate is logged.

## Migrated source detail

## Why

The task card's branch diff stat (`+N −M`) is meant to show what the task's
branch changed relative to its configured base branch. For workspaces that were
not created through a full agent launch, it instead reports a diff against
`origin/master`, producing numbers that can be four orders of magnitude too
large — and plausible-looking enough that a user may believe the branch really
touched thousands of files.

## Broken behavior

The per-repo base-branch map reaches agentctl's `WorkspaceTracker` through
exactly two paths:

1. `LaunchRequest` metadata, assembled by `collectBaseBranches` and stored under
   `MetadataKeyBaseBranches` — the **full launch** path only; and
2. `Manager.PushBaseBranchesForTask`, which fires only from
   `Service.UpdateRepositoryBaseBranch` when a user **edits** the base branch.

Every other way a workspace comes to exist skips both — notably
`startAgentOnExistingWorkspace` (the agent starting on a workspace that was
already prepared) and the workspace-only / lazy-recovery execution creation that
runs after a backend restart.

`WorkspaceTracker.BaseBranch()` is then empty, so `resolveBaseBranch` never
consults the stored value and falls through to `branchDiffCandidates`
(`origin/main`, `origin/master`, `main`, `master`). In any repository whose
integration line is not `main`/`master`, that resolves to an unrelated branch,
and `computeBaseCommit` returns a merge-base far back in history. The failure is
silent: no error, no warning, just a wrong number.

Observed in a local run against a repository whose integration branch is neither
`main` nor `master`. The task's configured base was correct in the database, its true diff
was `+66 −1`, and the card showed `+440545 −28354` — reproduced exactly as the
merge-base diff against `origin/master` (`+440466 −28354`) plus the two
untracked files' 79 lines. A sibling task in the same repository, whose
workspace *was* created through the full launch path, displayed correctly.

## What

- The stored per-repo base branch reaches the `WorkspaceTracker` for **every**
  workspace, regardless of how the workspace came to exist — full launch, agent
  start on an already-prepared workspace, or post-restart recovery.
- The branch diff stat is computed against the configured base branch whenever
  one is recorded for the task's repository.
- The integration-branch fallback (`origin/main` → `origin/master` → `main` →
  `master`) applies only when no base branch is recorded, which remains its
  intended purpose.
- When the tracker falls back to an integration candidate, that decision is
  observable in the logs, naming the repository and the candidate chosen. A
  silent fallback that yields a wrong-but-plausible number is not acceptable.
- Pushing the map is idempotent: re-sending an unchanged map is a no-op, and a
  workspace that already has the correct base is not disturbed.

## Failure modes

- The push fails (agentctl unreachable, transient error): logged at warn; the
  persisted `task_repositories.base_branch` remains the source of truth and the
  next workspace creation retries. Stats degrade to the existing fallback rather
  than breaking the workspace.
- No base branch is recorded for the repository: unchanged — the existing
  integration-branch fallback applies, now with a log line.
- The recorded base branch no longer exists in git: unchanged — `resolveStoredRef`
  fails to verify it and the fallback applies, now observable.
- A multi-repo task where only some repositories have a base branch: each
  repository resolves independently; the ones with a recorded base use it.

## Scenarios

- **GIVEN** a task whose repository has a recorded base branch, **WHEN** the
  agent starts on an already-prepared workspace, **THEN** the tracker resolves
  the diff base from that recorded branch, not from an integration candidate.
- **GIVEN** the same task, **WHEN** the backend restarts and the execution is
  recreated by lazy recovery, **THEN** the recreated workspace still resolves
  the recorded base branch.
- **GIVEN** a task whose repository has no recorded base branch, **WHEN** its
  workspace is created, **THEN** the integration-branch fallback applies as
  before and the chosen candidate is logged.
- **GIVEN** a workspace that already holds the correct base-branch map,
  **WHEN** the map is pushed again, **THEN** nothing changes and no error is
  raised.
- **GIVEN** a repository whose integration line is neither `main` nor `master`,
  **WHEN** a base branch is recorded, **THEN** the reported stat matches
  `git diff --numstat <merge-base with the recorded base>` plus untracked-file
  additions.

## Known gap

agentctl starts its workspace trackers during **instance creation**, and each
tracker runs one unconditional scan immediately — before the HTTP server the
backend waits on is reachable. The seeding push therefore cannot land before the
very first status computation, so a workspace created outside the launch path
can publish one fallback-based stat before its base branch arrives.

The window is bounded and self-healing: `SetBaseBranches` stamps the new ref
synchronously and kicks a background `RefreshGitStatus`, which emits a corrected
`GitStatusUpdate` over the same stream. So the wrong value is transient rather
than the indefinite one this repair fixes.

Closing it entirely means supplying the map at instance-creation time, or gating
tracker polling until hydration completes — a change to the instance-creation
contract, deliberately not bundled here.

## Out of scope

- Changing the integration-branch candidate list or making it configurable. The
  fallback is correct for its purpose; the defect is that it was reached at all.
- Changing how untracked-file additions are folded into the totals.
- The `correctStaleComparisonBase` re-pointing rule for merged/deleted stacked
  parents, which is a separate, intentional behavior.
- Backfilling or recomputing stats for workspaces that already exist; the value
  is not persisted and refreshes on the next poll.
