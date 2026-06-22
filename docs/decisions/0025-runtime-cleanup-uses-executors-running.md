# 0025: Runtime Cleanup Uses `executors_running`

**Status:** accepted
**Date:** 2026-06-22
**Area:** backend

## Context

Archiving and deleting tasks can remove task/worktree records while ACP agent
processes remain alive. Process inspection in a dev LXC container found many
`codex-acp` process trees reparented to PID 1 with current working directories
under deleted task worktrees. Most of those process trees were no longer
represented in the live `executors_running` table, which means Kandev had already
discarded its durable cleanup handle.

The existing archive/delete path builds stop targets from active
`task_sessions`, while runtime ownership is stored in `executors_running`. A
session can be terminal or missing from the active-session query while its runtime
row still points at a live process.

## Decision

Task archive/delete cleanup must derive runtime stop targets from
`executors_running` rows owned by the task before removing runtime tracking rows
or worktrees. `task_sessions.state` is user-facing session state; it is not the
source of truth for whether runtime resources still need cleanup.

Cleanup follows a fail-closed ordering:

1. Query the authoritative runtime inventory for the task from
   `executors_running`.
2. Attempt to stop every selected runtime by `agent_execution_id` or an available
   runtime-specific persisted handle.
3. Remove `executors_running` rows and worktrees only after stop succeeds or the
   runtime is positively confirmed absent.
4. Keep a retryable diagnostic row when stop cannot be confirmed.

Agentctl shutdown must also kill the owned agent process group when graceful stdin
EOF shutdown does not complete within the stop timeout, so agentctl cannot exit
while leaving ACP children reparented to PID 1.

## Consequences

**Easier:**

- Archive/delete cleanup no longer depends on active session state and catches
  terminal sessions that still own runtime resources.
- The durable runtime row remains available for retry and diagnosis when stop
  fails.
- Startup reconciliation can use the same inventory source to clean stale rows
  after a backend crash.

**Harder:**

- Cleanup code must preserve enough row state to retry instead of deleting
  `executors_running` unconditionally at the end of task cleanup.
- Tests need to cover terminal-session runtime rows, missing-session rows, and
  stop failures, not only active sessions.
- Runtime-specific fallback cleanup needs bounded behavior when the in-memory
  execution store no longer knows about the row.

## Alternatives Considered

- **Continue using active sessions and add more terminal cleanup hooks.** Rejected
  because it leaves multiple paths responsible for deciding whether a runtime is
  live. The durable ownership table is simpler and already exists.
- **Add an OS process sweeper for `codex-acp`/`claude-acp`/`opencode`.** Rejected
  as the primary fix because process-name scanning can kill unrelated user
  processes and does not address losing durable ownership before cleanup.
- **Delete runtime rows even when stop fails and rely on agentctl idle reaping.**
  Rejected because deleting the row removes the only authoritative handle Kandev
  has for retrying cleanup.
