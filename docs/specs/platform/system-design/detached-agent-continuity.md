---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-001
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-002
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-003
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-004
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-005
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-006
---

# Detached Agent Continuity System Design

## Purpose and boundaries

This design keeps a remote agent useful while its backend link is down. It
covers three parts:

- how the backend tracks a Disconnected execution;
- how it redials the executor transport;
- how agentctl behaves while no backend is attached.

Platform owns it because the behavior crosses every executor and extends the
shared recovery services.

It uses these contracts and does not own them:

- [Durable agent delivery](durable-agent-delivery.md): the agentctl journal,
  the `?after=` replay cursor, inbox projection, workflow effect keys,
  submission states, and `reconcileDisconnectedSubmission`. This design never
  resends a prompt and never replays a tool call.
- [SSH transport liveness](../../executors/system-design/ssh-transport-liveness.md):
  the keepalive watchdog that declares an SSH transport lost. This design
  starts after that declaration.
- [SSH reachability](../../executors/system-design/ssh-reachability.md): the
  per-executor reachability poller and its `executor.reachability.changed`
  event.
- [Agent survival across restart](../../executors/system-design/agent-survival-across-restart-01.md):
  agentctl's detached event retention and the unowned reaper. This design adds
  one invariant to that reaper.

The backend always initiates the link, and agentctl never dials the backend.
[ADR-2026-09-27-backend-dialed-detached-agents](../../../decisions/2026-09-27-backend-dialed-detached-agents.md)
records this and the other boundary choices.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-001` | [Disconnected link state](#disconnected-link-state), [Stop while disconnected](#stop-while-disconnected), [Orphaned agent after agentctl loss](#orphaned-agent-after-agentctl-loss) |
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-002` | [Redial contract](#redial-contract), [Reconnect coordinator](#reconnect-coordinator) |
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-003` | [Kandev tool calls while detached](#kandev-tool-calls-while-detached) |
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-004` | [Offline budget](#offline-budget) |
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-005` | [Agent guidance](#agent-guidance) |
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-006` | [Link events and notices](#link-events-and-notices), [Frontend](#frontend) |

## Components and responsibilities

| Component | Package | Responsibility |
| --- | --- | --- |
| Link state | `agent/runtime/lifecycle` | Records Disconnected on an `AgentExecution` instead of failing it |
| Reconnect coordinator | `agent/runtime/lifecycle` | Owns one reconnect loop per Disconnected execution and its triggers |
| `RemoteTransportRedialer` | `agent/runtime/lifecycle` | Executor contract that re-establishes a transport to the same agentctl |
| Executor redials | `executor_ssh*.go`, `executor_remote_docker.go`, `executor_sprites*.go`, `executor_kubernetes_refresh.go` | Per-transport redial |
| Detach clock | `agentctl/server/process` | Tracks the detached duration and enforces the offline budget |
| Waiting backend client | `mcp/server` | Holds Kandev tool calls while detached |
| Link notices | `orchestrator` | Writes the reconnect, turn-ended, and budget notices into the conversation |
| Disconnected banner | `apps/web/components/task/chat` | Shows the state and the Reconnect and Stop actions |

## Disconnected link state

`StreamManager.connectUpdatesStream` (`lifecycle/streams.go`) passes a
disconnect to `reconcileDisconnectedSubmission` first. If that does not
recover the stream, it calls `handleUpdatesDisconnectWithGeneration`. That
reaches `Manager.handleStreamDisconnectWithAttempt`
(`lifecycle/manager_events.go`), which today sets `v1.AgentStatusFailed`.

A new check runs before the failure branch. The disconnect enters Disconnected
when all of these hold:

- the execution's executor implements `RemoteTransportRedialer`;
- the disconnect is a transport or stream loss, not a process exit reported by
  agentctl;
- the execution is not being stopped.

Entering Disconnected does four things:

1. It records `LinkState{State: disconnected, Since, Host, BudgetDeadline}` on
   the execution. `BudgetDeadline` is `Since` plus the resolved offline budget.
   The session shows that time. It is a prediction, because agentctl enforces
   the budget on its own clock.
2. It keeps the execution tracked, and keeps its prompt-completion waiter
   pending, so the running turn is not completed or failed. The waiter
   resolves when replay delivers the turn's terminal event. If the agent is
   gone, it resolves through durable delivery reconciliation.
3. It publishes `events.AgentctlDisconnected` and persists the link state
   under the session metadata key `agent_link` (see [Persistence](#persistence)).
4. It starts the [reconnect coordinator](#reconnect-coordinator).

The task session state stays `RUNNING`, which is true on the executor host.
Disconnected is a link attribute, not a new `TaskSessionState`. Every existing
state consumer, filter, and workflow guard therefore stays unchanged. No timer
leaves Disconnected. The only exits are the three that
`AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.3` names.

An executor that does not implement `RemoteTransportRedialer` keeps the
current failure path. Local and worktree executors are in this group, because
their agentctl shares the backend host.

## Redial contract

The executor contract generalizes the Kubernetes refresher:

```go
// RemoteTransportRedialer re-establishes the transport to the agentctl that
// already runs instance, without starting a new one.
type RemoteTransportRedialer interface {
    RedialRemoteInstance(ctx context.Context, instance *ExecutorInstance) (*RemoteInstanceRefresh, error)
}
```

It returns the existing `RemoteInstanceRefresh` (`lifecycle/executor_backend.go`),
so the manager keeps a single staged-swap path. `Commit(publish)` installs the
new agentctl client under the execution write lock. `Abort` releases a
half-built transport. `ProcessRestarted` must be false for a redial. Any
executor-side evidence that agentctl was replaced returns
`ErrRedialTargetGone` instead.

The redial result is one of four outcomes:

| Result | Meaning | Coordinator action |
| --- | --- | --- |
| refresh | Transport re-established; the same agentctl answered its health check | Commit, then resume the stream |
| `ErrRedialUnreachable` | Host or transport cannot be reached | Stay Disconnected and schedule the next attempt |
| `ErrRedialTargetGone` | Host reached, but the recorded agentctl process no longer exists | Leave Disconnected through reconciliation |
| other error | Authentication, host-key, or configuration failure | Stay Disconnected, publish the error in the link state, back off at the cap |

Per executor:

- **SSH** (`executor_ssh.go`):
  - Redial takes the lost `sshSessionState` out of `SSHExecutor.sessions`.
    That entry currently makes `ResumeRemoteInstance` return early and
    `CreateInstance` return `ErrSSHTransportLost`.
  - It dials with the recorded target and pinned host-key fingerprint.
  - It probes the recorded agentctl `pid`. A dead pid yields
    `ErrRedialTargetGone`.
  - It opens a new forward with `StartPortForward`, which gives a new local
    port, then checks agentctl health with the recorded auth token.
  - It installs a fresh `sshSessionState`, with a new keepalive watchdog,
    under the same InstanceID.
  - The swap happens under `r.mu`, so a concurrent `CreateInstance` sees
    either the lost entry or the new one, never a gap.
- **Remote Docker** (`executor_remote_docker.go`):
  - The watchdog's `takeSession` deletes the session. The executor therefore
    keeps the dial target in `targets` when the loss is a transport loss.
  - Redial rebuilds the SSH client, the Docker client, and the forward from
    that target through the existing `reconnect` hook and
    `reconnectToContainer`.
  - A missing container yields `ErrRedialTargetGone`.
- **Sprites** (`executor_sprites*.go`): redial resolves the sprite with
  `reconnectSprite`, then opens a new proxy with `setupPortForwarding` and
  replaces the entry in `proxies`. A sprite that no longer exists yields
  `ErrRedialTargetGone`.
- **Kubernetes** (`executor_kubernetes_refresh.go`): `RedialRemoteInstance`
  delegates to the existing `RefreshRemoteInstance`. The 60-second
  `remoteStatusLoop` refresh stays in place. A port-forward drop now enters
  Disconnected first, instead of failing before the next refresh.

## Reconnect coordinator

The coordinator lives in `lifecycle` and owns at most one attempt at a time
per Disconnected execution. It uses `m.remoteRefreshGroup` so that it never
runs alongside a Kubernetes refresh.

Triggers:

| Trigger | Source | Delay |
| --- | --- | --- |
| Reachability returns | Subscriber to `events.ExecutorReachabilityChanged` with state `reachable`; matched to Disconnected executions by executor ID | Immediate |
| Backoff | Coordinator timer: 5 s, doubling, capped at 300 s, ±20% jitter, unlimited attempts | Scheduled |
| User Reconnect | WS action `session.reconnect` on a Disconnected session | Immediate; cancels the pending timer |
| Kubernetes status loop | Existing `refreshTrackedRemoteInstance` | Every 60 s |

An attempt runs these steps:

1. Call `RedialRemoteInstance`, bounded by 30 s.
2. On refresh, run `refreshTrackedRemoteInstance`'s commit steps, then
   `StreamManager.ReconnectAll(execution)`. `connectUpdatesStream` loads the
   projected cursor and reconnects with `StreamUpdatesFrom(after)`, so the
   journal replays what the agent produced while detached.
3. Run `reconcileDisconnectedSubmission` against the new client. Its result
   decides the turn: still running, completed in the journal, or uncertain.
4. Clear the link state, publish `events.AgentctlReady`, and record the
   reconnect notice.

A reconnect never calls `initializeAgentSession`, `LoadSession`, or any launch
intent. The same agentctl and harness process continue. A reconnect that finds
the agent gone hands over to durable delivery reconciliation, and the session
takes that outcome.

## Stop while disconnected

Stop on a Disconnected session does three things:

- it marks the session stopped;
- it records `pending_stop` in `agent_link`;
- it cancels the scheduled backoff, but keeps the reachability and
  user-Reconnect triggers.

On the next successful redial, the coordinator calls agentctl `agent.cancel`,
then stops the instance, before it resumes event intake. Admission stays
blocked in the meantime. If the agent is gone, nothing remains to stop and
the session stays stopped.

## Orphaned agent after agentctl loss

agentctl starts the agent in its own process group (`setAgentProcGroup`,
`agentctl/server/process/procattr_*.go`). On macOS and BSD, no parent-death
signal exists. `procattr_unix.go` states that orphan cleanup relies on
explicit `Stop()` calls. If agentctl exits, the agent keeps running there. It
can keep editing the workspace and pushing branches with nothing tracking it.

The design handles this in three places:

- **Record.** When agentctl starts the agent, it writes the process group ID
  (`Manager.AgentPID()`, because `Setpgid` makes the group ID equal the PID)
  and the process start time to `agent.pgid` in the session directory. For
  SSH, that directory is the one `ensureRemoteSessionDir` creates at
  `<taskDir>/.kandev/sessions/<sessionID>`. agentctl deletes the file when it
  stops the agent.
- **Reap on redial.** When a redial concludes `ErrRedialTargetGone`, the
  executor reads `agent.pgid` before it returns. If a process with that ID and
  the recorded start time is still alive, it sends `SIGTERM` to the group,
  waits 10 seconds, then sends `SIGKILL`. The start-time check stops a reused
  PID from being signalled. For remote Docker, the executor runs the same
  steps inside the container through `docker exec`, if the container still
  exists.
- **Report.** The executor returns `ErrRedialTargetGone`, with the reap result
  (`reaped`, `already_gone`, or `reap_failed`) as detail. The session outcome
  comes from durable delivery reconciliation only after the reap has finished
  (`AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.7`).

A `reap_failed` result is published in the link state and logged. The
coordinator does not retry the reap, because the host may now hold a
different process tree. The session shows the failure so that the user can
act on the host.

Local executors are outside this design. The same orphaning applies to them
on macOS, and it is tracked separately with the local agentctl death handling.

## Kandev tool calls while detached

`ChannelBackendClient.RequestPayload` (`mcp/server/backend_client.go`) hands a
request to the stream writer through the unbuffered `requestCh`, bounded by
`time.After(5 * time.Second)`. No writer reads `requestCh` while detached, so
every call fails.

The client takes an `AttachmentWaiter` from the process manager:

```go
type AttachmentWaiter interface {
    IsAttached() bool
    // Attached is closed when a stream attaches; BudgetExhausted when the offline budget ends.
    WaitSignals() (attached <-chan struct{}, budgetExhausted <-chan struct{})
}
```

The send step changes as follows:

- **While attached:** keep the 5 s bound. A stuck writer is still a local
  fault.
- **While detached:** wait on `requestCh`, `attached`, `budgetExhausted`, or
  `ctx`. When a stream attaches, retry the send. When the budget ends, return
  `ErrOfflineBudgetExhausted`.

Requests that were written but not answered when a stream dropped still fail
through `FailStreamRequests`, now with `ErrKandevCallOutcomeUnknown`. The
backend may already have applied them, and resending a non-idempotent call
could apply it twice.

Every handler that calls `RequestPayload` shares one keepalive wrapper:
`emitKeepAlivePings` at the 20 s `askQuestionKeepAliveInterval`, with a
progress token per call. Today only `ask_user_question_kandev` has it.

Claude runs with `MCP_TOOL_TIMEOUT=7200000`, a 2-hour limit
(`agents/claude_acp.go`). A budget above that would let the harness time out
a waiting call before the budget ends. The waiting client therefore holds a
call for no longer than the smaller of two values:

- the offline budget;
- the harness's declared tool timeout, minus 1 minute.

When that limit is reached, the call returns `ErrOfflineBudgetExhausted`.
Agents with no declared timeout, such as Codex, use the budget. The work
orders verify the Codex behavior.

Permission requests are not MCP calls. `sendPermissionNotification`
(`agentctl/server/process/manager.go`) already parks a request while
detached, and journal replay delivers it on reattach. This design keeps that
path. The only change is that budget expiry cancels the turn, which cancels
any parked request with it (`AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.5`).

## Offline budget

The detach clock in `agentctl/server/process` extends the `attachedCount`
counter in `attachment.go`:

- When `MarkDetached` brings the count to zero, it records `detachedSince` and
  arms a timer for the instance's `OfflineBudget`.
- `MarkAttached` stops the timer and clears `detachedSince`
  (`AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.4`).
- On expiry with a turn running, it calls the adapter's `Cancel` (the same
  `(*Adapter).Cancel` that `handleWSCancel` uses) and closes
  `budgetExhausted`.
- It journals an `agent_link.offline_budget_exhausted` event with its
  timestamp, so the backend learns of it through replay.

agentctl keeps running after expiry, so the journal remains readable.

The budget reaches agentctl in these steps:

1. The executor profile `Config` key `offline_budget_minutes` is added to
   `profileConfigAuthoritativeKeys` (`orchestrator/executor/executor_state.go`),
   like `ssh_reclaim_task_dir`.
2. `applyProfileConfigToMetadata` copies it into execution metadata.
3. Launch resolves it: 15 minutes when unset, rejected outside 1–1440.
4. It travels as `OfflineBudget` in `agentctl.CreateInstanceRequest`, which
   becomes `config.InstanceOverrides` and then the instance config.

No global setting and no feature toggle exist.

The unowned reaper (`cmd/agentctl/unowned_reaper_gate.go`) runs only with
agent survival on. It can shut agentctl down after `agentctl.unownedPeriod`
(10 minutes) without an ownership renewal, which is before a 15-minute budget.
`ownershipperiod.Resolve` gains a lower bound: the resolved period is at least
the largest active instance's offline budget plus 1 minute
(`AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.5`).

## Agent guidance

Error texts are stable Go constants in `mcp/server`:

- `ErrOfflineBudgetExhausted`: "Kandev is unreachable and the offline budget
  was reached. Stop now and end your turn. Do not poll, sleep, or retry this
  call."
- `ErrKandevCallOutcomeUnknown`: "The connection to Kandev dropped before this
  call returned. Its outcome is unknown. Check the current state before
  retrying."

`sysprompt.go` adds a `connectionLossSection` to the `kandev-context`
template (`config/prompts/kandev-context.md`), next to
`{step_complete_section}`. It states that Kandev tool calls can wait during a
connection loss, and that the agent must never wrap them in sleep or polling
loops.

## Link events and notices

| Event | Constant | WS action | Payload |
| --- | --- | --- | --- |
| Disconnected | `events.AgentctlDisconnected` (`agentctl.disconnected`) | `session.agentctl_disconnected` | `since`, `host`, `budget_deadline`, `next_attempt_at`, `last_error` |
| Reconnect attempt | same event, re-published | same | updated `next_attempt_at`, `last_error` |
| Reconnected | existing `events.AgentctlReady` | `session.agentctl_ready` | `reconnected_after_ms` |

These go through the existing `EventPublisher.PublishAgentctlEvent`
(`lifecycle/events.go`) and the gateway mapping in
`gateway/websocket/task_notifications.go`.

The orchestrator writes conversation notices as status messages:

- **Reconnected after a duration:** written when the coordinator clears the
  link state.
- **Turn ended while disconnected:** written when replay delivers a terminal
  event with a timestamp before the reconnect time. The workflow effect still
  runs once, through `workflowEffectForTurn` and
  `processOnTurnCompleteViaEngineWithCause`. The notice sits beside it and
  gates nothing.
- **Paused by the offline budget:** written when replay delivers
  `agent_link.offline_budget_exhausted`. The session then waits for input, and
  no queued prompt is dispatched automatically.

## Frontend

- `SessionAgentctlStatus.status` (`lib/state/slices/session/types.ts`) gains
  `"disconnected"`, with `since`, `host`, `budgetDeadline`, `nextAttemptAt`,
  and `lastError`.
- `lib/ws/handlers/agent-session.ts` handles `session.agentctl_disconnected`.
  On load, the session's `agent_link` metadata seeds the same status, which
  keeps state across a reload (`AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.6`).
- `DisconnectedSessionBanner` in `components/task/chat/` renders from
  `chat-input-container.tsx`. It takes precedence over `SessionStoppedBanner`
  while the status is `disconnected`. The shared `ChatInputArea` renders it on
  phones too.
- **Actions:**
  - Reconnect sends `session.reconnect`.
  - Stop uses the existing stop action.
  - Test IDs are `disconnected-session-banner`,
    `disconnected-reconnect-button`, and `disconnected-stop-button`.
- `RemoteCloudTooltip` and the kanban status icon show the disconnected
  state from the same status.
- Copy uses `task:` keys in all six locales, with no em dash:
  - `agentDisconnectedTitle`
  - `agentDisconnectedBody`
  - `agentDisconnectedPauseAt`
  - `agentReconnect`
  - `agentReconnectedAfter`
  - `agentTurnEndedWhileDisconnected`
  - `agentPausedByOfflineBudget`

## Failure and recovery

| Condition | Behavior |
| --- | --- |
| Host unreachable for hours | Disconnected; backoff at 5 min; budget cancels the turn at its deadline; reconnect later replays the cancellation |
| Host reachable, agentctl gone (host reboot, OOM) | Reap the orphaned agent group, then `ErrRedialTargetGone`; durable delivery reconciliation decides the outcome |
| Host-key mismatch or authentication failure | Stay Disconnected, show `last_error`, back off at the cap; never re-pin a host key automatically |
| Redial succeeds, stream replay fails (cursor invalid, journal lost) | Durable delivery's typed replay errors apply; the session takes its uncertain state |
| Backend restarts while Disconnected | The in-memory coordinator is gone; `agent_link` shows the session as Disconnected and the existing resume path applies on the next open |
| Two triggers fire together | Single-flight per execution; the second joins the first attempt |

## Persistence

`task_sessions.metadata.agent_link` stores:

- `state`
- `since`
- `host`
- `budget_deadline`
- `pending_stop`
- `last_error`

agentctl writes `agent.pgid` (process group ID and start time) in the session
directory on the executor host, and removes it when it stops the agent. The
coordinator writes `agent_link` when it enters Disconnected, on each attempt error
change, and clears it on reconnect or on a terminal outcome. It needs no new
table and no migration. The agentctl journal (durable delivery) keeps the
detached output and the budget event.

## Security

- A redial reuses the stored target, pinned host-key fingerprint, and agentctl
  auth token. It adds no credential and no inbound listener.
- A host-key mismatch is an error, never a trust decision.
- `session.reconnect` uses the same session-control authorization as
  `RetrySessionDelivery`.
- Error text shown to the agent carries no host, token, or path.

## Observability

Label sets are closed. No task, session, or host identifier appears in a
label.

| Metric | Labels |
| --- | --- |
| `agent_link_disconnected_total` | `executor_type` |
| `agent_link_reconnect_attempts_total` | `executor_type`, `trigger` (`reachability`, `backoff`, `user`, `k8s_refresh`), `outcome` (`reconnected`, `unreachable`, `target_gone`, `error`) |
| `agent_link_disconnected_seconds` (histogram) | `executor_type` |
| `agent_link_offline_budget_exhausted_total` | `executor_type` |
| `agent_link_orphan_reap_total` | `executor_type`, `outcome` (`reaped`, `already_gone`, `reap_failed`) |
| `agent_link_kandev_call_wait_total` | `outcome` (`answered`, `budget_exhausted`, `unknown_outcome`) |

Each transition also logs one structured zap line with the execution and
session IDs.

## Related decisions

- [ADR-2026-09-27-backend-dialed-detached-agents](../../../decisions/2026-09-27-backend-dialed-detached-agents.md)
- [Durable sessions across harness generations](../../../decisions/2026-09-10-durable-harness-session-boundaries.md)
