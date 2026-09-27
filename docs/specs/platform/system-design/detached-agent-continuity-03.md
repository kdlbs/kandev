---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-001
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-002
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-004
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-006
---

# Detached Agent Continuity System Design Part 3

## Purpose and boundaries

Part 3 of the design begun in [part 1](detached-agent-continuity-01.md). Part
1 states the purpose, the contracts this design uses, the Disconnected link
state, link generation, the redial contract, notices, persistence, and the UI.
[Part 2](detached-agent-continuity-02.md) covers agentctl while no backend is
attached. Part 3 covers how the backend leaves Disconnected:

- the reconnect coordinator, its triggers, and its attempt steps;
- rollback when an attempt fails after it installed a new client;
- Stop while Disconnected;
- a backend restart while a session is Disconnected.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-001` | [Stop while disconnected](#stop-while-disconnected), [Backend restart](#backend-restart) |
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-002` | [Reconnect coordinator](#reconnect-coordinator), [Attempt steps](#attempt-steps) |
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-004` | [Step 7: clear](#attempt-steps) (the budget queue hold) |
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-006` | [Attempt steps](#attempt-steps) (notice writes) |

## Reconnect coordinator

The coordinator lives in `lifecycle` and owns at most one attempt at a time
per execution in link state `disconnected` or `stopped_pending_cleanup`. It
uses `m.remoteRefreshGroup` so that it never runs alongside a Kubernetes
refresh. A trigger that arrives during an attempt joins that attempt and
starts nothing.

### Triggers

| Trigger | Source | When it runs |
| --- | --- | --- |
| Reachability returns | Subscriber to `events.ExecutorReachabilityChanged` with state `reachable`; matched to executions by executor ID | Immediately |
| Backoff | The coordinator's timer | At the scheduled time |
| User Reconnect | WS action `session.reconnect` on a `disconnected` session | Immediately |
| Kubernetes status loop | `refreshTrackedRemoteInstance` for an execution not in link state connected | Every 60 s |

### Backoff and timer rules

Each episode has one step counter `n`, starting at 0, and at most one pending
timer.

- The delay for step `n` is `min(5 s * 2^n, 300 s)`, multiplied by a jitter
  factor drawn uniformly from 0.8 to 1.2 for each scheduled timer. The
  jittered delay is never above 360 s and never below 4 s.
- Entering Disconnected sets `n = 0` and schedules the first timer. A new
  episode always starts at `n = 0`, whatever the previous episode reached.
- Every attempt that ends without a clear or a cleanup advances `n` by one,
  whatever triggered it, up to the step whose delay is 300 s. A joined
  trigger does not advance `n`, because it started no attempt.
- An immediate trigger (reachability, user, or Kubernetes) stops the pending
  timer before it starts its attempt. The attempt's end schedules exactly one
  new timer from the advanced `n`. An immediate trigger therefore never
  shortens or resets the backoff, and never leaves two timers.
- A result classed as "other error" in the [results
  table](detached-agent-continuity-01.md#results) sets `n` to the cap step,
  so the next timer is at the 300 s delay. The counter stays at the cap until
  an attempt ends with a different result class.
- The timer callback captures the episode generation. A callback whose
  generation is no longer current returns without an attempt.
- Clear, cleanup, and manager shutdown stop the timer. No timer outlives the
  execution's tracking.

`NextAttemptAt` in the link state is the pending timer's due time. It is nil
while an attempt runs.

## Attempt steps

An attempt captures the episode generation `G`. Every generation compare
below runs inside the same execution-lock hold as the write it guards, so
Stop, which also takes that lock, can never interleave between the compare
and the write. A compare that fails applies the rollback for the step
reached and returns.

1. **Redial.** Call `RedialRemoteInstance`, bounded by 30 s, with the
   identity from the execution's `DeliveryDescriptor`. Handle a non-refresh
   result as the [results table](detached-agent-continuity-01.md#results)
   says.
2. **Pending stop.** In link state `stopped_pending_cleanup`, run
   [the cleanup](#stop-while-disconnected) instead of steps 3 to 7.
3. **Commit.** Persist the new transport, as `refreshTrackedRemoteInstance`
   does today. Then `Commit(publish)` runs a guard inside its execution-lock
   hold, before it installs the client: the guard requires
   `LinkGeneration == G` and link state `disconnected`. On success it installs
   the client and sets `LinkGeneration = G+1`. The link state stays
   `disconnected` until step 7. On failure it installs nothing and the
   attempt calls `Abort`. A persisted transport record that was not committed
   is overwritten by the next redial. The coordinator does not call
   `StreamManager.ReconnectAll`.
4. **Replay to a barrier.** Read `GetDeliveryStatus` on the new client and
   store the descriptor on the execution. Its `HighWater` is the barrier.
   Call `ReplayRecoveredDelivery(ctx, execution)` synchronously. Replay pages
   over HTTP and connects no stream, so agentctl stays detached during it.
   Replay is complete when it returns nil, which means `DeliveryReplayCursor`
   has reached the barrier.
5. **Live stream.** Call the new synchronous
   `StreamManager.ConnectFromCursor(ctx, execution) error`. It wraps
   `connectUpdatesStream` and returns once `StreamUpdatesFrom(after)` has
   completed its handshake, or with the handshake error. The handshake is
   bounded by 90 s, because agentctl holds it while budget enforcement
   settles (see [part 2](detached-agent-continuity-02.md#budget-enforcement)).
   The new stream's disconnect callback captures generation `G+1`. Until step 7 clears, that
   callback does not enter Disconnected. It fails the attempt with a
   transport error, and the attempt rolls back.
6. **Settle the episode.**
   - Read `GetDeliveryStatus` again. agentctl is attached now, so its
     `AttachedAtSequence` (see [part 2](detached-agent-continuity-02.md#attachment-state))
     is the journal high water at the moment this stream attached. It is the
     episode's end sequence. A zero value means agentctl is detached again,
     and the attempt fails as a transport error.
   - Wait until the projected cursor reaches `AttachedAtSequence`, bounded
     by 30 s. The live stream delivers any event that agentctl journaled
     after the step 4 barrier but before the attach.
   - Classify the submission that was in flight, if any, with
     [`classifyReattachedSubmission`](#submission-classification).
7. **Clear.** Write the notices the episode recorded (see [part
   1](detached-agent-continuity-01.md#notices)), the reconnected notice last.
   Then, in one execution-lock hold, require `LinkGeneration == G+1` and link
   state `disconnected`, set link state `connected`, and read the episode's
   budget flag. Then persist and publish `events.AgentctlReady` with
   `reconnected_after_ms` and `budget_paused` set from that flag. The
   generation does not change here. If the compare fails, Stop won the race
   and the attempt rolls back.

### Submission classification

`classifyReattachedSubmission(ctx, execution, client)` is a new helper in
`lifecycle/durable_delivery_stream.go`. It reads
`client.GetDeliverySubmission` for `execution.deliverySubmissionIDSnapshot()`
and connects no stream. It runs after steps 4 to 6 have projected everything
agentctl journaled while detached. It does not call
`reconcileDisconnectedSubmission`, which keeps its current contract and its
single call site in `connectUpdatesStream`.

| Submission | Effect on the prompt-completion waiter |
| --- | --- |
| No submission ID on the execution (idle branch) | None; skip |
| `accepted` or `dispatching` | The turn is still running on the host. The waiter stays pending and the live stream resolves it |
| `completed`, `failed`, or `cancelled` with `TerminalEventRetained` and `TerminalSequence` at or below the projected cursor | Replay has already delivered the terminal event and resolved the waiter. No-op |
| `completed`, `failed`, or `cancelled` without a retained terminal event, or with `TerminalSequence` above the projected cursor | Uncertain |
| `prepared`, `interrupted_unknown`, or HTTP 404 (agentctl never journaled the submission) | Uncertain |
| Transport error | The attempt fails and rolls back |
| Any other error | Uncertain, with a warning log |

Uncertain resolves the waiter through the same marking that the prompt branch
of `handleStreamDisconnectWithAttempt` applies today: `FailureCode`
`DURABLE_DELIVERY_UNCERTAIN` with the submission ID as the detail. Durable
delivery's recovery controls own what happens next. A submission is never
resent, whatever its state.

Every waiter resolution is guarded by the prompt generation. A resolution
whose generation is no longer current, or whose waiter the live stream has
already resolved, is a no-op. The live stream and the classification can
therefore race without a double resolution.

### Failure handling

| Failure | Result |
| --- | --- |
| Redial result other than refresh | As the [results table](detached-agent-continuity-01.md#results) says |
| Commit fails, or its guard fails | `Abort`; no client was installed; stay in the current state |
| Any failure in steps 4 to 7 after the commit | [Rollback](#rollback-after-commit), then the next attempt on backoff |
| #3598 typed replay error in step 4 | The link is back. Durable delivery's typed error handling applies and the session takes its state. The link state becomes connected through step 7, with no rollback |
| Step 6 cursor wait times out | Rollback |
| A notice write fails in step 7 | Retried up to 3 times with a 1 s wait. Then the attempt rolls back and the notice is written by the next attempt under the same key |

### Rollback after commit

After step 3 has installed a new client, a failed attempt undoes the install
so that no later attempt finds a half-live transport:

1. In one execution-lock hold: if the execution's client is still the one
   this attempt installed, remove it. Then, if `LinkGeneration == G+1`, set
   `LinkGeneration = G+2`, keep link state `disconnected`, and set
   `last_error`. If the generation differs, Stop owns the link state and the
   link fields are left as they are.
2. Outside the lock: stop the new stream if step 5 started it, and close the
   client.
3. Call the new redialer method `DropRedialedTransport(instance)`. It tears
   down the transport state that `Commit` installed and marks the instance's
   transport as lost, so the next redial replaces it:
   - **SSH:** close the forward and the SSH client, and mark the
     `sshSessionState` entry lost, as the keepalive watchdog does. The next
     redial takes the lost entry.
   - **Remote Docker:** close the Docker and SSH clients and the forward, and
     keep the dial target in `targets`.
   - **Sprites:** close the proxy and remove it from `proxies`.
   - **Kubernetes:** stop the port-forward that the refresh opened.

   `DropRedialedTransport` is idempotent and never touches the agentctl
   process.

A rollback keeps the episode: its `EpisodeID`, its `Since`, and the notices
and budget flag it has recorded. Projection is idempotent per sequence, so the
next attempt's replay starts from the advanced cursor and repeats nothing.

A reconnect never calls `initializeAgentSession`, `LoadSession`, or any launch
intent. The same agentctl and harness process continue. A reconnect that finds
the agent gone hands over to durable delivery reconciliation, and the session
takes that outcome.

## Stop while disconnected

Stop on a Disconnected session ends Disconnected, which is the third exit in
`AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.3`. In one execution-lock hold it
sets link state `stopped_pending_cleanup` with `pending_stop` true and
increments `LinkGeneration` and `LinkRevision`. An attempt that has not
reached its step 3 or step 7 guard fails that guard. An attempt past step 3
rolls back, and its rollback leaves the link state to Stop. Then:

- The existing stop path moves the session to its stopped state. The
  execution stays tracked in the lifecycle manager, because its agent may
  still run on the host.
- The user Reconnect trigger is removed. The backoff, reachability, and
  Kubernetes triggers stay, and serve only the cleanup. The step counter
  continues from its current value.
- Any pending prompt-completion waiter resolves as cancelled, the same way a
  stop resolves it today.

A cleanup attempt runs step 1 of the coordinator, then:

1. If the redial returns a refresh, call agentctl `agent.cancel`, then stop
   the instance. No event stream is connected and no replay runs, so no
   admission or intake happens. The detached output stays in the journal and
   is removed with the instance.
2. If the redial returns `ErrRedialTargetGone`, nothing remains to stop.
3. After either, stop tracking the execution and write link state
   `cleared` with no pending stop. The session stays stopped.
   Reconciliation does not change a stopped session's outcome.
4. On `ErrRedialOrphanUnreaped` or a cancel or stop error, keep
   `stopped_pending_cleanup` with `last_error` and retry on the next trigger.

The chat shows the existing stopped banner, with one extra line while
cleanup is pending (see [part 1](detached-agent-continuity-01.md#frontend)).

## Backend restart

The coordinator, the episode, and the link revision counters live in memory.
A backend restart loses them. Reconnecting across a restart is out of scope,
so a restart ends every episode and hands the session to the existing restart
path:

1. **Sweep.** At startup, before `reconcileExecutorSessionsOnStartup`
   (`orchestrator/service.go`) recovers any execution, the orchestrator calls
   the new repository method `ClearStaleAgentLinks(ctx) (int, error)`. In one
   statement it rewrites every `agent_link` whose `state` is `disconnected` or
   `stopped_pending_cleanup` to `state: cleared` with `pending_stop` false,
   its stored `link_generation` and `link_revision` each plus one, and the
   other fields unchanged. It is dialect-aware for SQLite and PostgreSQL, like
   `SetSessionAgentLinkIfNewer`. A sweep failure is logged and counted, and
   startup continues.
2. **Seed.** When the lifecycle manager creates or adopts an execution for a
   session, it seeds `LinkGeneration` and `LinkRevision` from the stored
   `agent_link`, so its first link write is newer than the stored value and
   is not rejected by the revision guard.
3. **Existing path.** The session keeps its stored `TaskSessionState`, which
   is `RUNNING` for a Disconnected session and stopped for a pending cleanup.
   `reconcileOneSessionOnStartup` then applies its current rules: adoption
   when agent survival allows it, otherwise its current settlement. A session
   that was stopped pending cleanup stays stopped. The offline budget bounds
   its agent, and with agent survival on, the unowned reaper later ends its
   agentctl.

The frontend receives `cleared` from the session metadata on load, or from
any later link event, and removes the banner.

`session.reconnect` on a session whose execution is not tracked in link state
`disconnected` returns the typed `ErrAgentLinkNotDisconnected`. The gateway
returns it as a WS error with code `agent_link_not_disconnected`. The frontend
drops its stored link status for that session and reloads the session's
`agent_link` from the backend, so a banner left from before a restart
disappears.

## Related decisions

- [ADR-2026-09-27-backend-dialed-detached-agents](../../../decisions/2026-09-27-backend-dialed-detached-agents.md)
- [Durable sessions across harness generations](../../../decisions/2026-09-10-durable-harness-session-boundaries.md)
