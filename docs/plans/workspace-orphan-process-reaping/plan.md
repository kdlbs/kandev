---
created: 2026-09-09
status: complete
requirements:
  - REQ-TASKS-ORPHAN-REAP-001
  - REQ-TASKS-ORPHAN-REAP-002
  - REQ-TASKS-ORPHAN-REAP-003
  - REQ-TASKS-ORPHAN-REAP-004
  - REQ-TASKS-ORPHAN-REAP-005
  - REQ-TASKS-ORPHAN-REAP-006
  - REQ-TASKS-ORPHAN-REAP-007
system_design:
  - ../../specs/tasks/system-design/workspace-orphan-process-reaping.md
legacy_specs: []
---

# Implementation Plan: Workspace Orphan Process Reaping

## Overview

Close the gap between "the executions a task recorded" and "everything that
task left running." Terminal cleanup stopped the execution a task launched,
then removed the task's local workspace directories, but a process detached
into its own process group (for example, a background shell) survived both
steps and kept running with a cwd inside a now-deleted directory.

## Confirmed defect

On 2026-09-08 a card running an ad-hoc load-generator Bash tool call left
twelve shells at 1114% CPU, 18 hours after the card was archived and its
worktree removed. The cleanup job reported `succeeded`: its `resource_snapshot`
inventory (`sessions`, `worktrees`, `stop_targets`, `task_environment`) names
launched executions only, so it had no identifier that named the twelve
detached shells. The host reached load 29.97/50.96/70.23 on 14 cores, and three
unrelated SSH cards failed their ACP initialize handshake because of local
saturation, not remote health.

## Scope

### In scope

- Detect, after a workspace or task directory is removed, any host process
  still holding that path via its resolved cwd.
- Apply fail-closed ownership checks: never signal a PID belonging to a live
  `executors_running` row for another task, and never reap a shared
  `inherit_parent` workspace with a surviving active member.
- Escalate SIGTERM -> 2s grace -> SIGKILL per PID, matching the existing
  `runner.go` escalation used for launched executions.
- Report what was reaped from the cleanup job, so `succeeded` means what it
  says.

### Out of scope

- The ad-hoc load-generator script itself (not committed code).
- Remote SSH runner orphan reaping (tracked separately, card `f860757f`).

## Technical approach

### Detection

Snapshot host processes per platform: `lsof`/`ps` on darwin (no `/proc`
available), `/proc` on linux. Resolve each candidate's cwd rather than
scanning the directory tree, and resolve parent-process ancestry to support
ownership checks. Bound snapshot and candidate-count cost with an explicit
platform contract (`AC-TASKS-ORPHAN-REAP-007`).

### Scope and ownership

Reap by cwd under the removed path. Never signal a PID belonging to a live
recorded execution for another task, another task's live session workspace
(the effective path, not the raw column, per `REQ-TASKS-ORPHAN-REAP-003`'s
fail-closed reading), or a protected ancestor (the backend process itself).
Containment is checked bidirectionally against another task's live worktree,
one-directionally against the candidate's own worktree.

### Signal discipline

SIGTERM first, then SIGKILL after a grace period, matching `runner.go`'s
existing 2-second escalation. Recheck cancellation per-candidate, not just at
phase entry, so a cancelled cleanup does not falsely resolve a
not-yet-classified candidate.

### Reporting

Record every reap and every deliberate skip. The cleanup job's `succeeded`
result now reflects orphan-reap outcomes, not only the launched-execution
inventory.

## Tests

- `AC-TASKS-ORPHAN-REAP-001.1`-`.5`: a process whose cwd is inside a removed
  task workspace is terminated and recorded.
- `AC-TASKS-ORPHAN-REAP-003.1`-`.7`: a process in a different workspace, and a
  process belonging to another task's live execution, are both left alone; a
  shared workspace with a surviving active member is not reaped.
- `AC-TASKS-ORPHAN-REAP-007.1`-`.5`: cleanup still reports `succeeded` with no
  added latency worth measuring on an empty workspace.

Full AC-to-test traceability lives in
`docs/specs/tasks/system-design/workspace-orphan-process-reaping.md` and the 9
`resource_cleanup_orphan_reap*_test.go` files (87 top-level tests).

## Work orders

- [x] [Task 01: Reap orphaned host processes after workspace removal](task-01-reap-orphaned-workspace-processes.md)

## Verification results

- `cd apps/backend && go build ./...` passed.
- `go vet ./internal/task/...` passed.
- `go test -race -count=1 ./internal/task/service/... -run 'OrphanReap|Lsof|PSAncestry|ProcStat|ProcCwd|ReapPhase|ReapRoot|Reap'` passed (87/87 top-level tests).
- `golangci-lint run ./internal/task/...` reported 0 issues.
- Full pre-existing-failure baseline proven unrelated via a real merge-base
  scratch worktree (14 failing packages match exactly; the branch's own diff
  touches none of them).
- PR #3619 opened, reviewed (8 review rounds), and taken through PR fixup.

## Risks

- Host process enumeration differs by platform; an unresolvable candidate
  fails closed (skipped), not open (reaped).
- PID reuse without a generation/start-time check is an accepted, irreducible
  gap without pidfd (Spec Review round 1, refuted as unactionable).
