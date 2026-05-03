# 0003: executors_running as the Single Source of Truth for agent_execution_id

**Status:** accepted
**Date:** 2026-05-03
**Area:** backend

## Context

`agent_execution_id` was duplicated across three tables: `task_sessions.agent_execution_id`, `task_environments.agent_execution_id`, and `executors_running.agent_execution_id`. Each launch wrote all three via separate full-row UPDATEs, all racing against the in-memory `lifecycle.ExecutionStore`. The result:

- The orchestrator's `UpsertExecutorRunning` ran *after* the in-memory `executionStore.Add` but with a different ID source, so the DB and the runtime authority could disagree.
- Stale events from a defunct execution could overwrite the row's `resume_token` because the row had no concept of "which execution this token belongs to".
- A concurrent second `Launch` for the same session could pass the in-memory check-then-act guard and produce a phantom execution that no cleanup path could find.

The user-visible failure: tasks would show "Agent is not running" with two phantom sessions referencing IDs that no longer existed.

## Decision

Make `executors_running` the single durable source of truth for `agent_execution_id` and split column ownership by component:

- **Lifecycle manager owns** `agent_execution_id`, `container_id`, `runtime`, `status`. These are written exclusively by `lifecycle.persistExecutorRunning`, in lockstep with `executionStore.Add` (`internal/agent/lifecycle/persistence.go`).
- **Orchestrator owns** `resume_token`, `last_message_uuid`, and `metadata.context_window`. These are written via the new narrow CAS update `Repository.UpdateResumeToken(sessionID, expectedExecID, …)` keyed on `agent_execution_id`, so a stale event from a rotated execution is rejected with `models.ErrExecutionRotated` instead of clobbering the live token.

Schema migrations drop `agent_execution_id` and `container_id` from `task_sessions` and `task_environments` (recreate-table pattern, since the SQLite version we run pre-dates `DROP COLUMN`). Reads that previously joined on these columns now use a `LEFT JOIN executors_running` with `COALESCE(er.agent_execution_id, '')` so external callers still receive the same shape.

Concurrent-launch protection: `lifecycle.Manager.Launch` is wrapped in `golang.org/x/sync/singleflight` keyed on `sessionID`. Two parallel callers for the same session see one launch; the second observes the same execution.

Post-restart reconciliation **preserves** the executors_running row including `agent_execution_id`. The lifecycle manager's `persistExecutorRunning` UPSERTs the new ID atomically alongside the in-memory `Add` on the next launch. The stale ID is harmless during the idle window and serves as the "this session was previously launched" marker that drives `applyRunningRecordToResumeRequest → isResumedSession`. (Clearing it broke `ResumePassthroughSession` for TUI sessions — see Consequences.)

## Consequences

**Easier:**

- One writer for each column means there is no race to reason about — the row's `agent_execution_id` is always the in-memory store's truth.
- `storeResumeToken`'s narrow CAS makes "a stale event overwrites the live token" structurally impossible.
- `Launch` singleflight closes the duplicate-execution race without callers needing their own locks.
- The lifecycle code no longer reaches into `task_sessions.agent_execution_id` to detect "the agent rotated" — it asks the store.

**Harder:**

- Test fixtures that previously seeded `task_sessions.agent_execution_id` directly now have to go through `executors_running` (helper: `seedExecutorRunning` in tests).
- Anyone touching `executors_running` writes must respect the column-ownership split: lifecycle code uses `Upsert` (full row); orchestrator code uses the narrow CAS setters. Mixing the two re-introduces races.
- Post-restart, the `executors_running.agent_execution_id` value briefly references a defunct execution. Any code that treats this column as "the live execution" rather than "the previous launch marker" will misbehave. Read paths must consult the in-memory store first; the row is the durable mirror, not the runtime authority.

**Gotcha learned the hard way:** dropping `task_sessions.agent_execution_id` makes `ClearSessionExecutionID` into a foot-gun. Pre-refactor it cleared the redundant column; if a developer "migrates" it to clear `executors_running.agent_execution_id`, every backend restart silently regresses passthrough TUI resume because `applyRunningRecordToResumeRequest` flips `isResumedSession` to false → fresh-start path → no `--resume` flag → conversation context lost. The method was removed in this PR.

## Alternatives Considered

- **Telemetry-first phase.** Land observability for divergence events before the structural fix. Rejected — the bug was already in production, and the structural fix is small enough to land in one PR.
- **Keep the column in three tables, fix only the race.** Move the orchestrator's UPDATE into the lifecycle manager's transaction. Rejected — the race is the symptom; three sources of truth is the root cause. Future column adds would re-create the same problem.
- **Optimistic locking via a `version` column on `executors_running`.** A version bump on every write would also detect rotation. Rejected — coarser than CAS-on-`agent_execution_id` (would reject benign concurrent metadata updates), and requires every writer to handle the conflict explicitly.
- **Delete the row on restart and rely on lazy-launch.** Considered. Rejected — the row holds `resume_token` and worktree info needed for lazy recovery, and the post-restart "preserve" path is what makes `--resume` work for passthrough sessions on reconnect.
