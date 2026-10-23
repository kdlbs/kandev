---
status: draft
system: office
requirements:
  - REQ-OFFICE-RUN-SETTLE-001
  - REQ-OFFICE-RUN-SETTLE-002
  - REQ-OFFICE-RUN-SETTLE-003
---

# Office Run Settlement Integrity System Design

## Purpose and boundaries

Office owns run settlement: when a claimed run finishes, fails, or escalates.
The orchestrator owns session states and workspace inheritance; Office reads
them through the `task_session.state_changed` event and the session row. Dynamic
routing owns its failover decision; this design only waits for its outcome.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-RUN-SETTLE-001` | [Parked-session settlement](#parked-session-settlement) |
| `REQ-OFFICE-RUN-SETTLE-002` | [Inheritance resolution](#inheritance-resolution) |
| `REQ-OFFICE-RUN-SETTLE-003` | [Session verdict guard](#session-verdict-guard) |

## Components and responsibilities

- `office/service/run_session_park_settlement.go` subscribes to
  `task_session.state_changed`, resolves deferred failures, and settles parked
  launched sessions.
- `office/service/event_subscribers.go` shares one completion body
  (`completeClaimedTaskRun`) between lifecycle completion events and park
  settlement, and one failure body (`failClaimedTaskRun`) between immediate and
  deferred failures.
- `office/service/agent_error_session_verdict.go` holds the opt-in guard.
- `office/service/retry.go` classifies launch failures for retry or escalation.
- `orchestrator/handoff_inheritance.go` resolves the inherited environment.

## Data and contracts

- Park settlement reads `task_id`, `session_id`, `old_state`, `new_state`,
  `error_message`, `agent_profile_id`, and the committed `updated_at` from
  `task_session.state_changed`.
- The run is matched with `GetClaimedRunByTaskAndAgent` and settles only when
  `runs.session_id` equals the event's session and the run was already claimed
  when the park was recorded (`runs.claimed_at` is not after the park's
  `updated_at`). A run claimed after the park belongs to the next turn on the
  reused session and must not be finished by the previous turn's park.
- The guard reads `task_sessions.state` and `task_sessions.error_message`
  through `GetTaskSessionState`.
- The flag is `FeaturesConfig.OfficeAgentErrorSessionVerdict`, registered in
  `runtimeflags/registry.go` and set off in every profile.
- Office teardown and session launch/resume/stop serialize on the executor's
  per-session lock (`Executor.WithSessionOwnership`).

## Control flow

### Parked-session settlement

A successful turn on the per-launch session model publishes
`RUNNING -> WAITING_FOR_INPUT` with an empty error. Office loads the agent's
claimed run on the task, checks the launched session, and runs the AgentCompleted
completion body: finish, routing success, output summary, checkout release, and
runtime stamp. Office task sessions park at `IDLE` and keep their teardown path.

Park settlement runs for Office tasks too. A session parks at
`WAITING_FOR_INPUT` either with no IDLE teardown (Routine-workflow tasks, and
Office turns that never reach a stopping teardown: a cancelled turn, a session
without an agent execution, or a session relaunched for the next run) or just
before the Office IDLE teardown. The park is the one signal every turn
produces, so it settles the run and releases the agent ceiling; the teardown's
later `AgentStopped` is a no-op on the finished run.

`handleOfficeTurnComplete` decides ownership and runs the IDLE flip and
`StopAgent` while holding the executor's per-session launch/stop lock, the same
lock `LaunchPreparedSession`, resume, and session stop take. A successor
relaunch therefore cannot start between the ownership read and the stop; if it
already owns the lock, the completion waits, then observes the successor and
returns without touching its execution. The guard skips both actions, and still
claims the completion, when the session is `STARTING`, is `RUNNING` while the
completion snapshot was not, holds an agent execution other than the snapshot's,
or is running a turn other than the one that just completed. A relaunch that
reuses the live execution keeps the execution id and can leave the session
`RUNNING`, so its new turn is the signal that the session moved on. The IDLE
flip remains a compare-and-set against the completion snapshot's state.

### Inheritance resolution

`resolveInheritedEnvironmentChecked` rejects an archived parent first, then
tries, in order: the parent's primary session environment; the one environment
all parent sessions hold when no primary exists; the parent-owned task
environment; the workspace group environment. Each candidate passes the existing
existence and owner check. Sessions bound to different environments yield no
session candidate. No valid candidate keeps the unsafe-reuse refusal.

When nothing resolves, the refusal is marked `ErrInheritedWorkspacePending`
(still wrapped with `ErrWorkspaceReuseUnsafe`) only if the parent is live and no
source names any environment: no parent session holds one, the parent owns
none, and the workspace group has none. Any reference that failed to resolve,
or an archived or terminal parent, keeps the refusal permanent.

`HandleRunFailure` treats a permanent `ErrWorkspaceReuseUnsafe` like a
launch-validation error: the run fails and `escalateFailure` records the
`agent.error` activity and queues the coordinator escalation. A pending
refusal keeps the existing backoff; on `retry_count = 0` it appends a warn
`workspace.inheritance_pending` run event and a `run.workspace_pending`
activity entry carrying the reason.

### Session verdict guard

With the flag on, `handleAgentFailed` keeps its run event, working-state clear,
and post-start fallback, then parks the failure keyed by session ID. A parked
entry has one taker: the state handler, the bounded re-check timer, a later
AgentCompleted for the same run, or the immediate path when the session already
records a verdict. Only the event's own run identity may consume an entry: a
completion without a run identity (legacy task-and-agent fallback) or one
naming another run leaves it parked. While an entry is parked, a completion
without a run identity is ignored entirely: the fallback cannot identify the
completing run, so the run it resolves is settled only by the session verdict.
`sync.Map.CompareAndDelete` decides the taker.

| Session state | Verdict |
| --- | --- |
| `FAILED`, `CANCELLED`, errored `WAITING_FOR_INPUT` | failure: apply `failClaimedTaskRun` |
| `STARTING` (after the failure), clean `WAITING_FOR_INPUT`, `IDLE`, `COMPLETED` | superseded: discard |
| `CREATED`, `STARTING` or `RUNNING` read at failure time | pending: park |

## Failure and recovery

- A missing or unreadable session at failure time applies the failure at once.
- The re-check (default two minutes) applies only a recorded failure verdict; a
  still-live session keeps the run claimed for its current execution. A re-check
  read error is not evidence of a successor, so it applies the failure instead
  of discarding it.
- A parked failure does not survive a restart; the run stays claimed until
  stale-claim recovery requeues it.

## Persistence

No schema change. Parked failures live in memory.

## Security

No new trust boundary. Events and rows are internal.

## Observability

- Park settlement logs `settling Office run after its launched session parked`.
- A park older than the claimed run logs
  `ignoring park event for a run claimed after it was recorded`.
- A skipped late teardown logs
  `office turn complete — session relaunched, skipping stale teardown`.
- The guard appends `agent_error.deferred` and `agent_error.superseded` run
  events and logs both transitions.
- Unsafe workspace reuse appears as the run's `error` event, the `agent.error`
  activity, and the coordinator escalation.

## Related decisions

- [Runtime settings overrides](../../../decisions/0018-runtime-settings-overrides.md)
