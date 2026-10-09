---
created: 2026-09-17
status: done
requirements:
  - REQ-OFFICE-ASSIGN-RATE-001
  - REQ-OFFICE-ASSIGN-RATE-002
  - REQ-OFFICE-ASSIGN-RATE-003
system_design:
  - ../../specs/office/system-design/assignment-wake-rate-limit-01.md
---

# Implementation Plan: Office Assignment Wake Rate Limit

## Overview

Bound the number of agent-initiated `task_assigned` wakes one task can produce
in a rolling window. Removing the same-agent equality gate in
`ApplyTaskMutation` let an agent holding `can_assign_tasks` reassign a task to
itself repeatedly, spaced past the five-second coalescing window, minting an
unbounded stream of genuinely distinct wakes. Neither existing bound catches
this: the dedup key is generation-aware by design, and `task_assigned` is on the
attended-provenance allowlist, so the daily spend ceiling never applies.

## Scope

- One admission gate in `SchedulerService.queueRun`, after the recent-duplicate
  lookup and coalescing, before `CreateRun`. It binds the task-mutation
  producer, which is the only `task_assigned` producer routing through that
  function and the only one carrying an actor.
- A task-scoped window count derived from existing `runs` rows; no new table,
  column, migration, or background job.
- A new `QueueOutcome` value, declared in both places the enum lives.
- One `expvar` counter labelled by a closed three-value reason set, plus a Warn
  log at the decision site.

## Out of scope

- Dedup key identity, the coalescing window, and the 24-hour lookback.
- The event-subscriber producer of assignment wakes; a per-task allowance
  cannot bind it.
- The provenance classifier and the budget admission gates.
- Rate-limiting the assignment write itself; a refused wake still persists the
  assignee and bumps the generation.
- Any frontend. No screen renders the allowance, the counters, or a refusal.

## Implementation Wave

- [ ] [task-01-assignment-wake-rate-limit](task-01-assignment-wake-rate-limit.md)

## Verification

```bash
cd apps/backend && go test ./internal/office/scheduler ./internal/office/service \
  ./internal/runs/service ./internal/workflow/engine
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
```
