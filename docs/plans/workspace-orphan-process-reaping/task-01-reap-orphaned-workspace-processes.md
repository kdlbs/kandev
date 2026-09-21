---
id: "01-reap-orphaned-workspace-processes"
title: "Reap orphaned host processes after workspace removal"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ORPHAN-REAP-001
  - REQ-TASKS-ORPHAN-REAP-002
  - REQ-TASKS-ORPHAN-REAP-003
  - REQ-TASKS-ORPHAN-REAP-004
  - REQ-TASKS-ORPHAN-REAP-005
  - REQ-TASKS-ORPHAN-REAP-006
  - REQ-TASKS-ORPHAN-REAP-007
acceptance_criteria:
  - AC-TASKS-ORPHAN-REAP-001.1
  - AC-TASKS-ORPHAN-REAP-001.2
  - AC-TASKS-ORPHAN-REAP-001.3
  - AC-TASKS-ORPHAN-REAP-001.4
  - AC-TASKS-ORPHAN-REAP-001.5
  - AC-TASKS-ORPHAN-REAP-002.1
  - AC-TASKS-ORPHAN-REAP-002.2
  - AC-TASKS-ORPHAN-REAP-002.3
  - AC-TASKS-ORPHAN-REAP-002.4
  - AC-TASKS-ORPHAN-REAP-002.5
  - AC-TASKS-ORPHAN-REAP-002.6
  - AC-TASKS-ORPHAN-REAP-002.7
  - AC-TASKS-ORPHAN-REAP-003.1
  - AC-TASKS-ORPHAN-REAP-003.2
  - AC-TASKS-ORPHAN-REAP-003.3
  - AC-TASKS-ORPHAN-REAP-003.4
  - AC-TASKS-ORPHAN-REAP-003.5
  - AC-TASKS-ORPHAN-REAP-003.6
  - AC-TASKS-ORPHAN-REAP-003.7
  - AC-TASKS-ORPHAN-REAP-004.1
  - AC-TASKS-ORPHAN-REAP-004.2
  - AC-TASKS-ORPHAN-REAP-004.3
  - AC-TASKS-ORPHAN-REAP-004.4
  - AC-TASKS-ORPHAN-REAP-004.5
  - AC-TASKS-ORPHAN-REAP-004.6
  - AC-TASKS-ORPHAN-REAP-004.7
  - AC-TASKS-ORPHAN-REAP-005.1
  - AC-TASKS-ORPHAN-REAP-005.2
  - AC-TASKS-ORPHAN-REAP-005.3
  - AC-TASKS-ORPHAN-REAP-005.4
  - AC-TASKS-ORPHAN-REAP-005.5
  - AC-TASKS-ORPHAN-REAP-005.6
  - AC-TASKS-ORPHAN-REAP-005.7
  - AC-TASKS-ORPHAN-REAP-006.1
  - AC-TASKS-ORPHAN-REAP-006.2
  - AC-TASKS-ORPHAN-REAP-006.3
  - AC-TASKS-ORPHAN-REAP-006.4
  - AC-TASKS-ORPHAN-REAP-006.5
  - AC-TASKS-ORPHAN-REAP-006.6
  - AC-TASKS-ORPHAN-REAP-006.7
  - AC-TASKS-ORPHAN-REAP-006.8
  - AC-TASKS-ORPHAN-REAP-006.9
  - AC-TASKS-ORPHAN-REAP-007.1
  - AC-TASKS-ORPHAN-REAP-007.2
  - AC-TASKS-ORPHAN-REAP-007.3
  - AC-TASKS-ORPHAN-REAP-007.4
  - AC-TASKS-ORPHAN-REAP-007.5
system_design:
  - ../../specs/tasks/system-design/workspace-orphan-process-reaping.md
---

# Task 01: Reap orphaned host processes after workspace removal

## Summary

Add a reap phase to the durable `task_resource_cleanup_jobs` worker. After a
local workspace path is removed and confirmed absent, find any host process
whose resolved cwd is inside that path and terminate it (SIGTERM, then SIGKILL
after a grace period), per PID, with fail-closed ownership checks against
other tasks' live sessions and recorded executions.

## In scope

- Snapshot host processes (darwin `lsof`/`ps`, linux `/proc`) and resolve each
  candidate's cwd and parent-process ancestry.
- Match candidates to a removed workspace path by containment, resolving
  symlinks and falling back safely when resolution fails.
- Apply ownership checks: never signal a PID belonging to another task's live
  `executors_running` row, another task's live session workspace, or a
  protected ancestor (the backend process itself).
- Escalate SIGTERM -> 2s grace -> SIGKILL, per PID, matching the existing
  `runner.go` escalation.
- Record every reap and every deliberate skip, and report totals from the
  cleanup job instead of silently succeeding.
- Bound candidate and snapshot cost, and fail closed when host information is
  unresolvable.

## Out of scope

- The ad-hoc Bash load-generator script that caused the original incident
  (not committed code).
- Remote SSH runner orphan reaping (tracked separately).

## Acceptance

- A process whose cwd is inside a task workspace is terminated when that
  workspace is cleaned up, and the cleanup job records it.
- A process in a different workspace, and a process belonging to another
  task's live execution, are both left alone.
- A shared workspace with a surviving active member is not reaped.
- Cleanup still reports `succeeded` when there is nothing to reap, with no
  added latency worth measuring on an empty workspace.

## Verification

```bash
cd apps/backend
go build ./...
go vet ./internal/task/...
go test -race -count=1 ./internal/task/service/... -run 'OrphanReap|Lsof|PSAncestry|ProcStat|ProcCwd|ReapPhase|ReapRoot|Reap'
golangci-lint run ./internal/task/...
```

## Files likely touched

- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/resource_cleanup_orphan_reap*.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `docs/specs/tasks/requirements/workspace-orphan-process-reaping.md`
- `docs/specs/tasks/system-design/workspace-orphan-process-reaping.md`

## Dependencies

None.

## Risks

- Host process enumeration differs by platform (darwin `lsof`/`ps` vs. linux
  `/proc`); an unresolvable candidate must fail closed (skip), not fail open
  (reap).
- Ancestry walks can be spoofed by PID reuse; irreducible without pidfd,
  accepted as a known gap.

## Parallelism

`sequential`

## Inputs

- `docs/specs/tasks/requirements/workspace-orphan-process-reaping.md`
- `docs/specs/tasks/system-design/workspace-orphan-process-reaping.md`
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`

## Results

Done. PR #3619 implements the reap phase exactly as specified, including the
frozen spec's nine accepted-open gaps (each with its safe reading recorded in
the system design doc). 87 top-level tests across 9
`resource_cleanup_orphan_reap*_test.go` files pass under `-race`; the existing
regression suite is unaffected (verified against a merge-base baseline).

### Process snapshot race regression

The Linux process snapshot regression now covers a process directory that is
present during enumeration but disappears before its `stat` record is read.
The scanner skips that unrelated exited PID while still failing closed for
partial stat data, permission failures, and malformed records.

Verification:

- `go test ./internal/task/service -run 'OrphanReap' -count=1` passed.
- `go test -race ./internal/task/service -run 'OrphanReap' -count=1` passed.
- The focused normal and race process-snapshot cases passed.
- `git diff --check` passed.
