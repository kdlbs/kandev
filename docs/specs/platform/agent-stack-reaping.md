---
status: implemented
created: 2026-08-23
updated: 2026-08-24
owner: platform
---

# Agent stack reaping

## Why
Kandev launches one ACP agent stack (agentctl instance + agent subprocess tree, 100-310 MB RSS each) per session and never terminates it when the surrounding work goes idle: a normal turn completion leaves the ACP process alive waiting for the next prompt, and `agent.completed` (which does trigger teardown) only fires when the process itself exits. Over days, idle-but-alive stacks accumulate until a memory-hungry turn pushes the machine into swap thrash. Users on long-lived installations lose the machine to I/O storms caused by nothing but abandoned idle agents.

The 2026-08-23 incident is the reference case: 11 ACP stacks launched between Aug 20 and Aug 22, every one of them belonging to a task sitting in REVIEW, still alive two to three days later and holding ~3-3.5 GB of mostly swapped-out RSS. A single task had accumulated five stacks of its own across agent switches. A frontend build inside a task workspace then took 2.1 GB, filled the 2.9 GB swap, and drove a 6-vCPU / 9.8 GB host to a load average of 70 for 75 minutes with the CPU 95% idle. This demonstrates the accumulation failure mode, but its machine size and workload do not establish a universal process-count ceiling for other Kandev installations.

## What
- Reaping MUST NOT fire on the task REVIEW transition. `setSessionWaitingForInput` writes REVIEW after *every* completed turn, so a REVIEW trigger is not idle reclamation: it removes warm-stack reuse outright. Each follow-up prompt would then pay a process relaunch plus ACP `session/load` replay, lose the provider prompt cache, and — for an agent whose `SessionConfig.SupportsRecovery()` is false (Auggie) or that does not advertise the ACP `LoadSession` capability — fall back to re-injecting the whole transcript. Idleness is measured, not inferred from a turn ending.
- When a task reaches COMPLETED the orchestrator stops the agent stacks of that task's idle sessions. This is the only lifecycle trigger, and it is not redundant with `Executor.StopByTaskID`: that call only reaches CREATED/STARTING/RUNNING/WAITING_FOR_INPUT sessions, so IDLE (office fire-and-forget) and COMPLETED sessions keep a live stack, and workflow terminal-step completion never stopped agents at all.
- A safety net reaps any remaining idle agent stack after the operator-configured `agentctl.idleTimeout` (default 1 hour) without session activity, regardless of how it was left behind (office tasks, uncovered paths, races). Idle age MUST be measured from the session row, not from `executors_running.updated_at` — the latter is refreshed by execution persistence and status writes, so a long-lived stack that just finished a turn would read as ancient. A zero timeout disables this safety net without disabling task-COMPLETED cleanup.
- Stopping a stack MUST NOT lose the session: resume token, worktree, and message history survive, and the next prompt on that session relaunches a fresh stack via the existing lazy-resume path.
- Reaping MUST never stop an agent mid-turn, a session in a working state, a session with an active turn record, or a session with a prompt inside its admission window (between `ensureSessionRunning` and `claimPromptDispatch`, where the session row still reads settled but the execution is already spoken for). Every guard is fail-closed: an uncertain signal (turn service unavailable, state lookup error) means skip, not force.
- Task-triggered sweeps MUST be owned by the service: they run on a tracked worker with a shutdown-cancellable context that `Service.Stop` joins before returning.
- A runtime feature toggle (`features.agentStackReaping`, env `KANDEV_FEATURES_AGENT_STACK_REAPING`) gates the behavior. It defaults to OFF in every shipped profile while experimental; operators can enable it for controlled deployments via Settings > System > Feature Toggles or the environment variable (restart required).

## API surface
- Runtime flag registry entry `features.agentStackReaping` (see `docs/specs/platform/feature-toggles.md` for the registry contract). Metadata: feature kind, experimental, medium risk, restart required, mutable.
- The existing `agentctl.idleTimeout` startup setting also supplies the orchestrator-side settled-session timeout when the flag is enabled. No new HTTP/WS/CLI surface is added. Teardown uses the existing `StopAgentWithReason` lifecycle path; observable as the existing `agent.stopped` event and `stopping agent` log lines carrying reason strings `agent stack reaping: task completed` and `agent stack reaping: idle ttl`.

## State machine
The task COMPLETED transition (existing) gains a side effect; no new states. REVIEW is deliberately untouched.

```text
turn ends ──> session settles (WAITING_FOR_INPUT / COMPLETED)
          ──> task CAS IN_PROGRESS/SCHEDULING → REVIEW succeeds
          ──> stack stays warm (no reaping side effect)

task COMPLETED transition ──> detached stop of idle sessions' stacks:
                 flag off? skip
                 session working? skip
                 active turn? skip
                 prompt in admission? skip
                 no live execution? nothing to do
                 else graceful StopAgentWithReason
```

Idle net (periodic, every 30 s alongside the idle-row reaper):

```text
session idle ≥ agentctl.idleTimeout (session row clock)
  + session in {WAITING_FOR_INPUT, IDLE, COMPLETED}
  + no active turn, no prompt in admission
  + live execution exists
  ──> same guarded stop; row repair happens on the next existing reclaim tick
```

## Failure modes
- Stop failure (agentctl unreachable or runtime teardown failure): logged at warn and kept in lifecycle tracking. No stopped event is published until the runtime confirms teardown, and the idle net retries on later ticks. The existing row-repair/reclaim machinery converges the `executors_running` row once the process is confirmed dead.
- New prompt races a sweep: the admission marker makes the reaper skip the session for the whole prompt, so the prompt cannot lose its execution to reaping. If a stack is already gone when the prompt arrives, the existing prompt path lazy-resumes or falls back to a fresh launch (`TestPromptTask_LazyResumeExecutionNotFoundFallsBackToFreshLaunch`); the user-visible effect is a relaunch, not an error.
- Turn service unavailable: the stop is skipped (fail-closed); a later tick with the service available retries.
- Shutdown during a sweep: the sweep context is cancelled and `Service.Stop` joins the worker before tearing down the repo and agent manager.
- Flag disabled: no stop is scheduled anywhere; behavior is exactly pre-fix.

## Persistence guarantees
Nothing new persists. Resume tokens, worktrees, sessions, messages, and turns are untouched by the stop; only the in-memory execution entry and the live process tree are torn down. After backend restart the startup reconciliation and idle-row reaper continue to behave as before.

## Scenarios
- **GIVEN** a task in IN_PROGRESS whose session finished a turn, **WHEN** the task state CAS to REVIEW succeeds, **THEN** no stop is issued and the stack stays warm for the follow-up prompt.
- **GIVEN** a task whose state changes to COMPLETED with an idle session stack, **WHEN** the persistent task transition succeeds, **THEN** the session's stack receives a graceful `StopAgentWithReason` with reason `agent stack reaping: task completed` and the stopped event preserves the COMPLETED session state.
- **GIVEN** a task completed by the user with an IDLE session that `StopByTaskID` does not reach, **WHEN** `CompleteTask` runs, **THEN** the COMPLETED sweep stops that session's stack.
- **GIVEN** `StopTask` whose REVIEW state write fails, **WHEN** the stop runs, **THEN** the agents are still stopped and the state-write error stays non-fatal.
- **GIVEN** a task transitioning to COMPLETED while another of its sessions is RUNNING, **WHEN** the sweep runs, **THEN** the working session's stack is not stopped (session-state guard).
- **GIVEN** a session with an active turn record or a prompt inside its admission window, **WHEN** any trigger evaluates it, **THEN** no stop is issued.
- **GIVEN** `features.agentStackReaping` disabled, **WHEN** a task completes or the idle tick fires, **THEN** no stop call is made.
- **GIVEN** a session whose session row has been settled for longer than `agentctl.idleTimeout` with a live execution and no active turn, **WHEN** the idle tick evaluates it, **THEN** the stack is stopped with reason `agent stack reaping: idle ttl`.
- **GIVEN** a session whose `executors_running` row is hours old but whose session finished a turn seconds ago, **WHEN** the idle tick evaluates it, **THEN** the stack is left alone.
- **GIVEN** a session row older than `agentctl.idleTimeout` whose `executors_running` row was recently refreshed, **WHEN** the idle tick evaluates it, **THEN** the stack is still stopped because executor-row age is not the idle clock.
- **GIVEN** a session whose stack was stopped by any path, **WHEN** the user sends a follow-up prompt, **THEN** the prompt path relaunches a fresh stack (lazy resume; fresh-launch fallback when the resume token is stale) and the turn proceeds normally.
- **GIVEN** a task sweep in flight, **WHEN** `Service.Stop` runs, **THEN** the sweep context is cancelled and Stop does not return until the sweep worker has finished.

## Out of scope
- Changing the agentctl-side idle reaper's activity signal (an open `/agent/stream` WebSocket currently counts as activity there); the orchestrator-side idle net makes that path redundant for stack reaping.
- Memory/cgroup limits, zram, nightly restarts, disk hygiene, and alerting. Those are the host-level remediations from the same incident report and are tracked outside this repository; the nightly-restart timer in particular is explicitly disposable once this ships.
- A universal live-stack count or memory ceiling. The incident host's six-stack budget is deployment-specific; Kandev does not infer a safe cross-platform limit without broader operational evidence.
- Office-specific lifecycle hooks (office tasks skip the REVIEW write today; the idle net covers their stacks).

## Implementation plan
[`docs/plans/agent-stack-reaping/plan.md`](../../plans/agent-stack-reaping/plan.md)
