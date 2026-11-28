---
id: "04-archived-recovery-fences"
title: "Preserve archived recovery fences"
status: done
wave: 4
depends_on:
  - "01-backend-archive-gate"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.7
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
---

# Task 04: Preserve Archived Recovery Fences

## Summary

Close open recovery blocks only for archived tasks with terminal sessions,
retaining their delivery outcome and evidence. Reopen archive-only closures in
the same ordinary or cascade unarchive transaction before eligibility is visible.
Keep Office runs parked until actual session recovery authorizes release.

## In scope

- Task-owned recovery action identity, archived-owner eligibility and row locking.
- Bounded archived-session reconciliation and idempotent archive-only closure.
- Ordinary and cascade unarchive with atomic reopening.
- Delivery retention protection for archive-only closed recovery evidence.
- Office recovery-park consumers distinguishing bookkeeping from authorization.
- The existing recovery requirement and system design.

## Out of scope

No automatic resume, prompt replay, workspace replacement, outcome fabrication,
retention policy changes, operating database repair or new recurring procedure.

## Acceptance

1. Only archived tasks with terminal sessions close their open blocks.
2. Live-task and active-session blocks remain open.
3. Unarchive restores the original fence and evidence before recovery admission.
4. Archive-only closure never clears an Office run's recovery reference.
5. Actual explicit recovery still reenters the existing Office admission path.
6. PostgreSQL closure and both unarchive paths serialize on the owning task row.

## Files likely touched

- apps/backend/internal/task/models/session_continuity.go
- apps/backend/internal/task/repository/sqlite/session_continuity.go
- apps/backend/internal/task/repository/sqlite/task.go
- apps/backend/internal/task/repository/sqlite/agent_delivery_retention_prompts.go
- apps/backend/internal/task/repository/sqlite/agent_delivery_retention_proof.go
- apps/backend/internal/task/repository/sqlite/archived_recovery_block_test.go
- apps/backend/internal/task/repository/sqlite/agent_delivery_retention_prompt_test.go
- apps/backend/internal/task/service/archived_session_reconciliation.go
- apps/backend/internal/task/service/archived_session_reconciliation_test.go
- apps/backend/internal/office/service/scheduler_integration.go
- docs/specs/agents/requirements/agent-resume-runtime-recovery.md
- docs/specs/agents/system-design/agent-resume-runtime-recovery.md

## Verification

Reuse the existing archive, cascade-unarchive, delivery-retention and scheduler
checks. Disposable Go overlays exercise archive closure, reopen and explicit
recovery of the same Office run, plus real PostgreSQL multi-connection ordering.
These temporary cases stay outside automatic test collection and are not added
as a permanent suite. Record actual execution and limitations below.

## Results

- A disposable Office overlay reproduced the incorrect park release before the
  guard, then passed after the fix. The same run stayed parked through archive
  bookkeeping and reopened state; genuine explicit recovery still reentered
  normal budget admission. The existing explicit-recovery scheduler control
  also passed.
- Existing SQLite archive/cascade/live-session checks passed with the race
  detector. Existing real PostgreSQL ordinary and cascade unarchive/reorder
  locking checks passed with the race detector.
- A disposable real PostgreSQL probe used three independent pooled connections
  and a held task-row lock. Both close-first and unarchive-first orderings passed
  for ordinary and cascade unarchive; all four completed with an active task,
  an open original fence and unchanged delivery evidence. The final ordinary
  cases and the unchanged successful cascade cases were verified in targeted
  runs.
- The probe's activity observer refreshes its transaction-local statistics
  snapshot before checking waiters. Its ordinary fixture uses ordinary archive;
  cascade ownership is tested only through the matching cascade API.
- PostgreSQL ran in an isolated localhost-only container using the cached
  postgres:16 image. Temporary credentials were not logged; containers stopped
  and removed after each run. No operating database was used.
- The existing documentation coverage evaluator reports covered with no errors
  for every triggering source path.
- Temporary probe files remain outside automatic collection; no permanent test
  suite or new verification procedure is introduced.

No operating state was changed.
