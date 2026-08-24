---
spec: docs/specs/platform/agent-stack-reaping.md
created: 2026-08-23
updated: 2026-08-24
status: implemented
---

# Implementation Plan: Agent Stack Reaping

## Overview

Terminate idle ACP agent stacks at two triggers: (1) the task COMPLETED
transition and (2) an idle safety net inside the existing idle-session reaper
that stops any stack whose *session* has been settled for the operator's
`agentctl.idleTimeout` with no active turn. Both share one fail-closed stop
primitive gated by the experimental `features.agentStackReaping` runtime flag,
which defaults off in every shipped profile.

## Root cause recap

A normal turn completion settles the session to WAITING_FOR_INPUT and the task
to REVIEW, but the ACP process stays alive waiting for the next prompt.
`agent.completed` — the only event that tears the stack down
(`handleAgentCompleted` → `cleanupAgentExecution` → `StopExecution`) — fires
only when the process itself exits. Idle stacks therefore accumulate for days.

## Key design decisions

- **REVIEW is not a trigger.** The first cut of this fix hooked
  `writeTaskReviewState` / `writeTaskReviewStateOnCancel`. Those are the
  every-turn path (`setSessionWaitingForInput`), so the hook did not reap idle
  stacks — it deleted warm-stack reuse. Every follow-up prompt would pay a
  relaunch plus `session/load` replay, lose the provider prompt cache, and for
  an agent with `SupportsRecovery() == false` (Auggie) or no ACP `LoadSession`
  capability fall back to re-injecting the transcript. The interactive
  follow-up window is precisely the window the configurable idle net protects, so
  measuring idleness replaced inferring it.
- **COMPLETED is a trigger, and is not redundant.**
  `Executor.StopByTaskID` lists only CREATED/STARTING/RUNNING/
  WAITING_FOR_INPUT sessions, so IDLE (office fire-and-forget) and COMPLETED
  sessions survive `StopTask` / `CompleteTask` with a live stack, and
  `markTaskCompletedForTerminalStep` never stopped agents at all. The COMPLETED
  sweep closes exactly those gaps.
- **Stop/Complete ordering is unchanged.** The first cut reordered `StopTask`
  to write REVIEW before stopping agents and made the write fatal, which meant
  a failed state write returned an error *and* left the agent running on an
  explicit user stop. Both methods keep their original ordering; the COMPLETED
  sweep is additive.
- **Service-owned sweeps.** Task-triggered sweeps run on `agentStackSweeper`,
  a tracked worker pool with a shutdown-cancellable context that `Service.Stop`
  joins, rather than a bare `go` statement whose blocked `StopAgentWithReason`
  could outlive shutdown. The sweep context is `WithoutCancel` of the service
  context (a sweep must outlive the triggering request) re-wrapped in
  `WithCancel` (but not shutdown).
- **Prompt-admission interlock.** `ensureSessionRunning` releases the
  per-session lifecycle lock on return and `claimPromptDispatch` only re-takes
  it later, so between them the session reads settled-with-no-active-turn while
  its execution is already spoken for. A sweep landing there stopped the
  execution, and because the execution existed before `ensureSessionRunning`,
  `resumedForPrompt` was false and the fresh-launch fallback did not apply — the
  prompt simply failed. `beginPromptAdmission` marks the window and the reaper
  refuses to stop a marked session.
- **Idle clock is the session row.** `executors_running.updated_at` is
  refreshed by execution persistence and status writes, so a long-lived stack
  that finished a turn seconds ago read as ancient. `sessionIdleSince` reads
  `task_sessions.updated_at`, falling back to `started_at` and finally the
  executor row.
- **Deployment-neutral policy.** The incident's six-stack budget reflected one
  host's memory and workload, not a safe Kandev-wide limit. The universal cap
  was removed; timeout cleanup uses the existing operator-configurable
  `agentctl.idleTimeout` (default one hour).
- **Shared primitive.** `stopIdleSessionAgentStack` (`agent_stack_reaper.go`)
  implements the guards and is the only caller of `StopAgentWithReason` for
  reaping. Graceful stop (`force=false`), matching `completeAndStopSession`.
- **Turn re-entry (verified, not assumed).** `promptTask` calls
  `ensureSessionRunning` before dispatch; it lazy-resumes a WAITING_FOR_INPUT
  session with no live execution, and the prompt path has an explicit
  dead-execution → fresh-launch fallback
  (`TestPromptTask_ReentersAfterAgentStackReaping`,
  `TestPromptTask_LazyResumeExecutionNotFoundFallsBackToFreshLaunch`).
- **Staged rollout.** The flag is experimental, mutable, restart required, and
  off in prod/dev/e2e by default. Controlled installations can opt in before
  the behavior is promoted.
- **Confirmed cleanup.** `StopAgentWithReason` retains lifecycle tracking and
  returns an error when runtime teardown fails. Removal and `agent.stopped`
  publication happen only after the runtime confirms the stop, so later ticks
  can retry instead of losing the execution handle.

## Tasks

- task-01: runtime flag `features.agentStackReaping` across profiles, config,
  registry, orchestrator ServiceConfig, and frontend defaults.
- task-02: shared guarded stop primitive, service-owned sweeper, prompt-
  admission interlock, and the task COMPLETED trigger.
- task-03: configurable idle safety net on the session clock, independent of executor-row age.
- task-04: regression tests (guards, flag off, all triggers, REVIEW-stays-warm,
  turn re-entry, shutdown join) and full backend/web verification.

## Verification state

- Passed: focused backend tests for orchestrator, lifecycle, runtimeflags, config, and profiles, including retryable runtime teardown, executor-age-independent TTL scanning, deterministic prompt/reaper interleaving, and turn re-entry.
- Passed: `make -C apps/backend lint`.
- Passed: web feature-contract test, ESLint, standalone TypeScript typecheck, and public-doc validation.
- Passed: `KANDEV_BACKEND_PORT=19876 CGO_ENABLED=1 GOCACHE=/tmp/kandev-go-cache make -C apps/backend test lint`. The isolated port avoids the existing Kandev process occupying `10101`; the temporary Go cache keeps the verification run within the available workspace disk.
