---
id: "02-pool-workspaces-fences"
title: "Pool workspaces, fences and quarantine"
status: done
wave: 2
depends_on:
  - "01-pool-placement-transport"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NOMADPOOL-003
  - REQ-EXECUTORS-NOMADPOOL-004
  - REQ-EXECUTORS-NOMADPOOL-005
  - REQ-EXECUTORS-NOMADPOOL-006
acceptance_criteria:
  - AC-EXECUTORS-NOMADPOOL-003.1
  - AC-EXECUTORS-NOMADPOOL-003.2
  - AC-EXECUTORS-NOMADPOOL-003.3
  - AC-EXECUTORS-NOMADPOOL-004.1
  - AC-EXECUTORS-NOMADPOOL-004.2
  - AC-EXECUTORS-NOMADPOOL-004.3
  - AC-EXECUTORS-NOMADPOOL-005.1
  - AC-EXECUTORS-NOMADPOOL-005.2
  - AC-EXECUTORS-NOMADPOOL-005.3
  - AC-EXECUTORS-NOMADPOOL-006.1
  - AC-EXECUTORS-NOMADPOOL-006.2
system_design:
  - ../../specs/executors/system-design/nomad-worker-pool.md
---

# Task 02: Pool workspaces, fences and quarantine

## Summary

Run several environments per node safely, with the right workspace on each kind of node and cleanup that touches only what it owns.

## In scope

- Local workspaces as git worktrees of Kandev's checkout; remote workspaces as fresh single-branch checkouts like the SSH executor; resume affinity with workspace-unavailable on loss.
- Per-environment start fence replacing the worker-wide fence; audit every per-worker assumption in launch, recovery, drain and cleanup.
- Small default reservations with `memory_max_mb`; preflight detects disabled memory oversubscription.
- Per-node quarantine marker and eligibility; other nodes keep placing.
- `TMPDIR` and agent scratch inside the allocation directory; proven cleanup removes the worktree or checkout, uncertain cleanup retains it with the quarantine inventory.

## Out of scope

- Live pool proof and handoff (task 03); phase-2 items.

## Acceptance

- Two environments on one node stay isolated through stop, recovery and cleanup of either.
- Quarantine of one node leaves placement on the others working; no replacement allocation is ever created.

## Verification

Focused Go tests for fence paths, two-environment isolation, workspace strategy, quarantine scope and scratch paths; runner offline tests for the corresponding pool scenarios.

## Files likely touched

- `apps/backend/internal/agent/nomad/bootstrap.go`, `quarantine.go`, `job.go`
- `apps/backend/internal/agent/runtime/lifecycle/`
- `apps/backend/internal/task/repository/sqlite/executor_nomad_inventory.go`

## Dependencies

Task 01.

## Risks

Per-worker assumptions hidden in cleanup or recovery paths; audit before changing the fence.

## Parallelism

None.

## Inputs

- [System design](../../specs/executors/system-design/nomad-worker-pool.md)

## Results

Complete (2026-10-26). Offline and ssh-node live proof done (`docs/evidence/nomad-pool-p2/`, build a1c7afce42): per-environment fence, TMPDIR scratch, workspace removal on proven cleanup, per-node quarantine scope. Local-node git worktree (REQ-003.1) is implemented with offline tests (`workspace_local_worktree.go`, `WorkspaceRemoveCommand`); its live proof is in P3's two-node run.
