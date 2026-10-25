---
status: active
system: workspaces
created: 2026-08-05
owners:
  - Kandev
---
# Workspace Base-Branch Propagation Requirements

## Overview

Workspace Git statistics and commit history describe changes on the task branch relative to each repository's configured comparison base. Recovery must preserve that comparison from the first result so unrelated integration history cannot appear as task changes.

## Requirements

### REQ-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001: Workspace Base-Branch Propagation

**Intent:** Workspace Git statistics and commit history describe changes on the task branch relative to each repository's configured comparison base. Recovery must preserve that comparison from the first result so unrelated integration history cannot appear as task changes.

#### Acceptance criteria

- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.1:** The stored per-repo base branch reaches the `WorkspaceTracker` for **every** workspace, regardless of how the workspace came to exist — full launch, agent start on an already-prepared workspace, or post-restart recovery.
- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.2:** The branch diff stat is computed against the configured base branch whenever one is recorded for the task's repository.
- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.3:** The integration-branch fallback (`origin/main` → `origin/master` → `main` → `master`) applies only when no base branch is recorded, which remains its intended purpose.
- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.4:** When the tracker falls back to an integration candidate, that decision is observable in the logs, naming the repository and the candidate chosen. A silent fallback that yields a wrong-but-plausible number is not acceptable.
- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.5:** Pushing the map is idempotent: re-sending an unchanged map is a no-op, and a workspace that already has the correct base is not disturbed.
- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.6:** **GIVEN** a task whose repository has a recorded base branch, **WHEN** the agent starts on an already-prepared workspace, **THEN** the tracker resolves the diff base from that recorded branch, not from an integration candidate.
- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.7:** **GIVEN** the same task, **WHEN** the backend restarts and the execution is recreated by lazy recovery, **THEN** the recreated workspace still resolves the recorded base branch.
- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.8:** **GIVEN** a task whose repository has no recorded base branch, **WHEN** its workspace is created, **THEN** the integration-branch fallback applies as before and the chosen candidate is logged.
- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.9:** When a workspace execution is recreated and its recorded base branches are available, its first comparison-derived result shall use those branches, including results produced before readiness is announced.
- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.10:** The commits result shall contain only task-branch commits relative to each repository's configured comparison base. A task with no commits ahead of its recorded bases shall return zero commits, including after recovery; repositories with different bases shall resolve independently.

- **AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.11:** Creation-time best-effort hydration shall stop after at most five seconds, honor an earlier caller deadline or cancellation, and continue workspace request preparation without a hydrated map on lookup failure.

## Failure and compatibility behavior

- Hydration or delivery failure remains best-effort and observable in warnings;
  workspace access is preserved and the next workspace creation retries.
- A recorded branch that no longer exists retains the existing observable
  integration fallback. This repair does not change missing-ref handling.
- Mixed inventories resolve each repository independently. Configured bases do
  not suppress intentional fallback for an unconfigured sibling.

## Out of scope

- Changing or configuring the integration-branch candidate list.
- Making successful hydration a mandatory workspace-access gate.
- Changing untracked-file arithmetic or stale stacked-parent correction.
- Persisted-stat backfills or commit-list truncation presentation.

## System design and implementation plans

- [Workspace base-branch propagation design](../system-design/workspace-base-branch-propagation.md)
- [Original propagation repair](../../../plans/workspace-base-branch-propagation/plan.md)
- [Recovered execution base-branch repair](../../../plans/recovered-execution-base-branches/plan.md)
