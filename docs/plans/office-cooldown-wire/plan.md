---
spec: docs/specs/office/agents.md
created: 2026-08-22
status: done
---

# Implementation Plan: Wire the Office agent cooldown gate

## Overview

PR #2902 fixed `office_agent_runtime.last_run_finished_at` to be stamped correctly on every
completed/stopped run (sync and async, task-bound and taskless), but explicitly scoped out two
follow-ups. This plan closes both, resolved together because they're the same underlying question
("what does the Office heartbeat cooldown gate mean?"):

1. **Wrong column.** `heartbeatAgentRuntime.AllowFire` (`internal/backendapp/cron.go`) reads
   `agent_profiles.last_run_finished_at` — a column from the shared kanban agent-settings table
   that no Office write path ever stamps — instead of `office_agent_runtime.last_run_finished_at`,
   the column `Service.stampRunFinished` actually maintains. `agent.LastRunFinishedAt != nil` is
   therefore always false and the cooldown branch never fires: Office heartbeat cooldown is
   silently a no-op today, regardless of `cooldown_sec`.
2. **Missing stamp on failure.** `HandleAgentFailure` (`internal/office/service/failure.go`) marks
   a run failed and releases its task checkout but never calls `stampRunFinished`. Once (1) is
   fixed, a below-threshold failing agent would be exempt from cooldown pacing entirely (unlike a
   successful or stopped run), letting a flapping agent retry with zero pacing until it crosses the
   auto-pause threshold.

Resolved spec decision (see amendments below): cooldown paces **every** terminal run outcome for an
agent — completed, stopped, taskless-completed, and failed alike — uniformly. It is a run-pacing
mechanism, not a success tracker. The separate consecutive-failure counter / auto-pause threshold
remains the mechanism for stopping a chronically broken agent; cooldown only paces the failures that
happen before that threshold is reached.

Out of scope: removing or migrating the dead `agent_profiles.last_run_finished_at` column (bigger
blast radius across the shared kanban agent-settings table; not needed to fix the bug — the gate
just needs to stop reading it). No schema change. No frontend change.

## Spec amendments (already applied)

- `docs/specs/office/agents.md` — persistence-guarantees bullet for `office_agent_runtime` now
  names the exact gate (`heartbeatAgentRuntime.AllowFire`), states which column it must read, and
  explicitly calls out the `agent_profiles` same-named column as dead/not-authoritative.
- `docs/specs/office/runtime.md` — "Retry policy" section states that a terminal failure stamps the
  cooldown timestamp same as success; a new persistence-guarantee bullet and a new GIVEN/WHEN/THEN
  scenario cover it.

## Backend

### Task 01: Cooldown gate reads `office_agent_runtime`, not `agent_profiles` — done

`heartbeatAgentRuntime.AllowFire` currently gates on `agent.LastRunFinishedAt` (from
`office.GetAgentInstance`, i.e. `agent_profiles`). Change it to load the runtime row via
`office.GetAgentRuntime(ctx, agentID)` (already exists, returns `*officesqlite.RuntimeState` with
`LastRunFinishedAt *time.Time`, `nil, nil` when no row) and gate on that timestamp instead.
`agent.CooldownSec` still comes from `agent_profiles` (that field is actively written and unrelated
to this bug). No repository or schema change — `GetAgentRuntime` already exists and is already
exercised by other tests.

### Task 02: `HandleAgentFailure` stamps the cooldown timestamp — done

Add a `s.stampRunFinished(ctx, run)` call to `HandleAgentFailure` (`internal/office/service/failure.go`),
placed after `s.releaseTaskCheckoutForRun(ctx, run)` and before the consecutive-failure counter
work, mirroring the completed/stopped paths' ordering (finish/mark → release checkout → stamp).
`stampRunFinished` is already shared, idempotent, and no-ops safely on a nil run or empty
`AgentProfileID`; this task is a one-line call plus a test.

## Task waves

| Wave | Tasks | Parallel-safe |
|------|-------|----------------|
| 1 | 01, 02 | Yes — disjoint files (`internal/backendapp/cron.go` vs `internal/office/service/failure.go`), disjoint test files, no shared schema/migration/generated contract. |

Default `parallelism: sequential` in each task file per repo convention; the table above is a human
dispatch aid, not standing authorization to delegate to subagents.

## Validation

```bash
cd apps/backend && go test ./internal/backendapp/... ./internal/office/service/... ./internal/office/repository/sqlite/...
cd apps/backend && golangci-lint run ./internal/backendapp/... ./internal/office/service/... --timeout=5m
```
