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
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-004` | [Step 7: clear](#attempt-steps) (the stream confirm that restarts the budget, and the budget queue hold) |
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-006` | [Attempt steps](#attempt-steps) (notice writes, the clear deadline), [Typed replay error](#typed-replay-error) |

## Reconnect coordinator

The coordinator lives in `lifecycle` and owns at most one attempt at a time
per execution in link state `disconnected` or `stopped_pending_cleanup`. It
uses `m.remoteRefreshGroup` so that it never runs alongside a Kubernetes
refresh. A trigger that arrives during an attempt joins that attempt and
starts nothing.

An attempt owns the execution from its start until the execution-lock hold
that ends it: the step 7.3 hold of a clear, the step 3 hold of a [typed
replay error](#typed-replay-error), the step 1 hold of a
[rollback](#rollback-after-commit), or the end of a cleanup. Work after that
hold, such as steps 7.4 and 7.5, runs outside the attempt. A trigger that
arrives then starts a new attempt instead of joining, so a disconnect during
step 7.4 or 7.5 gets its own attempts and timer.

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

- The delay for step `n` is `min(5 s * 2^n * j, 300 s)`, where `j` is a
  jitter factor drawn uniformly from 0.8 to 1.2 for each scheduled timer. The
  clamp applies after the jitter, so a delay is never above 300 s, the
  5-minute cap of `AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.2`, and never
  below 4 s. The delay at `n = 0` is 4 s to 6 s, and at `n = 6` it is 256 s
  to 300 s.
- The **cap step** is `n = 7`, the first step whose delay is exactly 300 s
  for every jitter draw, because 5 s * 128 * 0.8 is above 300 s.
- Entering Disconnected sets `n = 0` and schedules the first timer. A new
  episode always starts at `n = 0`, whatever the previous episode reached.
- Every attempt that ends without a clear or a cleanup advances `n` by one,
  whatever triggered it, up to the cap step, and never past it. A joined
  trigger does not advance `n`, because it started no attempt.
- An immediate trigger (reachability, user, or Kubernetes) stops the pending
  timer before it starts its attempt. The attempt's end schedules exactly one
  new timer from the advanced `n`. An immediate trigger therefore never
  shortens or resets the backoff, and never leaves two timers.
- A result classed as "other error" in the [results
  table](detached-agent-continuity-01.md#results) sets `n` to the cap step,
  7. `n` never decreases within an episode, so every later timer in the
  episode is at the cap delay, whatever the later results are.
- A timer captures the link generation current when it is scheduled, read
  under the execution lock, and gets a new timer token that the execution
  stores as its pending token in the same hold. Stopping the timer clears the
  pending token. A callback takes the execution lock and returns without an
  attempt unless its token is the pending token and its generation is still
  current; otherwise it clears the pending token as its attempt starts. A
  callback that had already fired when its timer was stopped therefore
  starts nothing, and never leaves a second timer. Stop while Disconnected increments the
  generation, so in the same lock hold it stops the pending timer, if any,
  and schedules one timer at the delay for the current `n` that captures the
  new generation (see [Stop while disconnected](#stop-while-disconnected)).
  While an attempt runs, no timer is pending, and the attempt's end
  schedules the next timer with the generation current at that end.
- Clear, cleanup, a typed replay error, and manager shutdown stop the
  timer. An attempt that ends with a clear, a cleanup, or the typed replay
  branch's step 3 schedules no timer. No timer outlives the execution's
  tracking.

`NextAttemptAt` in the link state is the pending timer's due time. It is nil
while an attempt runs.

## Attempt steps

An attempt captures the episode generation `G`. Every generation compare
below runs inside the same execution-lock hold as the write it guards, so
Stop, which also takes that lock, can never interleave between the compare
and the write. A compare that fails applies the rollback for the step
reached and returns.

From step 5 until it clears or rolls back, the attempt also holds an
**attempt record** on the execution, under the execution lock: its
`attach_id`, an `ended` flag, and an outcome channel. The record decides
every race between the attempt stream and the attempt:

- Step 5 sets the record, with `ended` false, before it dials.
- The attempt stream's disconnect callback takes the execution lock. If a
  record with its `attach_id` exists, it sets `ended` true and fails the
  attempt as a transport error: it cancels the attempt's context, so the
  step in progress returns and the attempt rolls back. It does not enter
  Disconnected. If no
  record exists, the callback takes the normal generation rule in [part
  1](detached-agent-continuity-01.md#link-generation): after a clear it
  enters Disconnected, and after a rollback it is stale and dropped.
- Step 7.2 and step 7.3 require `ended` false in their lock holds.
- Step 7.3, when its guard holds, removes the record and closes the outcome
  channel as cleared, in the same lock hold. A rollback removes the record
  and closes the outcome channel as rolled back, in its step 1 lock hold.

So a stream that ends before step 7.3 always fails the attempt, and a stream
that ends after it always enters Disconnected. At most one attempt record
exists per execution, because attempts are single-flight.

The backend holds each Kandev MCP request that arrives on a stream whose
`attach_id` matches the attempt record. The request waits on the outcome
channel, or until the stream's context ends, and is not dispatched. Cleared: the request is dispatched as usual.
Rolled back: the request is dropped without an answer, and the rollback's
stream close makes agentctl fail it with `ErrKandevCallOutcomeUnknown` (see
[part 2](detached-agent-continuity-02.md#agent-guidance)). The backend never
processed it, and the error text tells the agent to check state before it
retries. The hold is bounded by the clear deadline in step 6. So no Kandev
call is processed for a session whose Stop won, as
`AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.6` requires.

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
5. **Live stream.** Generate a new `attach_id` (a UUID) for the attempt and
   call the new synchronous
   `StreamManager.ConnectFromCursor(ctx, execution, attachID) error`. It
   returns once the handshake has completed, or with the handshake error. The
   handshake is bounded by 90 s, because agentctl holds it while budget
   enforcement runs (see [part 2](detached-agent-continuity-02.md#budget-enforcement)).
   The stream is unconfirmed until step 7, so agentctl stays detached.
   - `connectUpdatesStream`'s body moves into a new
     `connectUpdatesStreamErr(dialCtx, execution, ready, attachID) error`,
     which returns the error it logs today. `dialCtx` bounds only the dial;
     the stream's lifetime keeps using `sm.streamContext(execution)`.
     `StreamUpdatesFrom` gains the dial context and the `attach_id` as
     parameters and adds `attach_id` to the stream URL when it is not empty.
   - `connectUpdatesStream(execution, ready)` keeps its signature and becomes
     a call to `connectUpdatesStreamErr` with no `attach_id`. Its callers,
     `connectUpdatesStreamAsync`, the overload retry in
     `shouldReconnectAfterStreamOverload`, and `ConnectAll`, do not change.
   - `ConnectFromCursor` calls `connectUpdatesStreamErr` with a 90 s dial
     context and the `attach_id`. The cursor comes from
     `deliveryReplayCursor`, as for every other stream, so it is at the step 4
     barrier.

   The new stream's disconnect callback captures generation `G+1` and the
   `attach_id`. While the attempt record exists, that callback does not
   enter Disconnected. It fails the attempt with a transport error, and the
   attempt rolls back.
6. **Settle the episode.**
   - Read `GetDeliveryStatus` again. Its `Attachment` (see [part
     2](detached-agent-continuity-02.md#attachment-state)) must report
     `Current` true with this attempt's `attach_id`. Otherwise the stream has
     ended or been superseded, and the attempt fails as a transport error.
     `AttachedAtSequence` is then the journal high water at the moment this
     stream became current, and the episode's end sequence. Zero is a valid
     end sequence for an empty journal.
   - If `Attachment.UnjournaledBudgetPause` is set, agentctl could not
     journal its budget event (see [part
     2](detached-agent-continuity-02.md#budget-enforcement)). The episode
     records it as a budget event at `AttachedAtSequence`, so the budget
     flag and the notice rules of [part
     1](detached-agent-continuity-01.md#notices) apply to it as to a
     journaled one, and it sorts after every journaled event at the same
     sequence. Its notice key is
     `agent-link-budget-unjournaled:<session_id>:<episode_id>:<exhausted_at>`,
     with `ExhaustedAt` in Unix milliseconds, so a later attempt that reads
     the same pause again writes nothing new, and a second unjournaled pause
     in the same backend episode gets its own notice.
   - Wait until the projected cursor reaches `AttachedAtSequence`, bounded
     by 30 s. The live stream delivers any event that agentctl journaled
     after the step 4 barrier but before the stream became current. When
     this wait succeeds, replay is complete in the sense of
     `AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.3`, and a 3 s **clear
     deadline** starts. It bounds the rest of step 6 and step 7 up to the
     step 7.3 lock hold. Each notice write and the confirm call in that span
     retries after 250 ms on an error that is neither a transport error nor
     a 409, until the deadline. When the deadline passes before the lock
     hold, the attempt rolls back. Step 7.4 then takes at most 2 s before
     it publishes. A reconnect that succeeds therefore always publishes its
     clear within 5 s of replay completing.
   - Classify the submission that was in flight, if any, with
     [`classifyReattachedSubmission`](#submission-classification).
7. **Clear.** In this order:
   1. Write the turn-ended and budget notices the episode recorded, in
      ascending journal sequence (see [part
      1](detached-agent-continuity-01.md#notices)). They record journal
      facts, so they stay true if Stop wins below.
   2. In one execution-lock hold, require `LinkGeneration == G+1`, link
      state `disconnected`, and `ended` false. If any fails, the attempt
      rolls back without confirming. Then, outside the lock, confirm the
      stream: `POST /api/v1/agent/stream/confirm` with the
      attempt's `attach_id`. Stop can still win after that check. Kandev
      calls that the confirm releases are then held and dropped, as the
      attempt record states. agentctl is attached from here, and the offline
      budget restarts at its next detach. A 409 `ATTACH_NOT_CURRENT` means
      the stream has ended or been superseded: the attempt fails as a
      transport error and rolls back, and the budget has not restarted.
      A confirm whose response is lost may have reached agentctl; the
      rollback's close then starts a new episode with a full budget. A
      budget restart therefore always needs a confirm that reached agentctl,
      so attempts that keep failing in steps 4 to 6 never restart it.
   3. In one execution-lock hold, require `LinkGeneration == G+1`, link
      state `disconnected`, and `ended` false; set link state `connected`,
      read the episode's budget flag, and remove the attempt record as
      cleared. The generation does not change here. If the generation or
      state compare fails, Stop won the race. If `ended` is true, the stream
      has already gone. Either way the attempt rolls back.
   4. Persist and publish `events.AgentctlReady` with
      `reconnected_after_ms`, and `budget_paused` set from that flag. The
      SQL write is bounded by 2 s. The publish runs after it returns, fails,
      or times out, as [part 1](detached-agent-continuity-01.md#write-order)
      states for a failed write. A write that timed out is logged and
      counted like a failed one, and the next link write repairs it.
   5. Write the reconnected notice. It is written only after the guard in
      3 held, so it never records a reconnect that Stop overtook. A write
      error is retried up to 3 times, 1 s apart, then logged and counted as
      `agent_link_write_failed_total{target="notice"}`. The link stays
      connected. A backend crash between 3 and 5 loses the notice with the
      rest of the in-memory episode, as [Backend restart](#backend-restart)
      states.

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
| Transport error: a dial or connection error, a context deadline or cancellation (including the clear deadline), or an HTTP 5xx or 429 response | The attempt fails and rolls back. The next attempt classifies again |
| Any other error: an HTTP 4xx response other than 404 and 429, or a response body that does not decode | Uncertain, with a warning log |

Uncertain resolves the waiter through the same marking that the prompt branch
of `handleStreamDisconnectWithAttempt` applies today: `FailureCode`
`DURABLE_DELIVERY_UNCERTAIN` with the submission ID as the detail. Durable
delivery's recovery controls own what happens next. A submission is never
resent, whatever its state.

Every waiter resolution is guarded by the prompt generation. A resolution
whose generation is no longer current, or whose waiter the live stream or a
Stop has already resolved, is a no-op. A classification that finishes after
Stop resolved the waiter as cancelled therefore leaves it cancelled. The live stream and the classification can
therefore race without a double resolution.

### Failure handling

| Failure | Result |
| --- | --- |
| Redial result other than refresh | As the [results table](detached-agent-continuity-01.md#results) says |
| Commit fails, or its guard fails | `Abort`; no client was installed; stay in the current state |
| Any failure in steps 4 to 7.3 after the commit | [Rollback](#rollback-after-commit), then the next attempt on backoff |
| #3598 typed replay error in step 4 | [Typed replay error](#typed-replay-error); no rollback |
| Other replay error in step 4 | Transport class: rollback. Anything else: rollback with `last_error` `replay` |
| Step 6 cursor wait times out | Rollback |
| Clear deadline passes before step 7.3 | Rollback. The next attempt writes the same notice keys, so nothing duplicates |
| Confirm returns 409 in step 7.2 | Rollback. The budget has not restarted |
| Step 7.2 or 7.3 guard fails (Stop won, or the attempt stream ended) | Rollback. Held Kandev calls are dropped unprocessed |
| Step 7.4 SQL write fails or passes 2 s | Logged and counted; the publish still runs |
| Reconnected notice write fails in step 7.5 | Logged and counted; the link stays connected |

### Typed replay error

A #3598 typed replay error is one that matches row 4 of the [classification
table](detached-agent-continuity-01.md#disconnect-classification): cursor
behind retention, stream identity mismatch, sequence error, or journal error.
The link works, but durable delivery cannot continue the stream, so the
episode ends through durable delivery's existing path instead of a clear:

1. Steps 5 and 6 do not run. No live stream is connected, no `attach_id` is
   created, and nothing is confirmed, so agentctl stays detached and its
   budget keeps running.
2. Write the turn-ended and budget notices for the recorded events at or
   below the projected cursor, in ascending journal sequence. No reconnected
   notice is written, and no budget flag is read, because no queue dispatch
   follows. Each notice write uses the step 7.5 policy: up to 3 retries,
   1 s apart, then a log and a count of
   `agent_link_write_failed_total{target="notice"}`. The branch then goes on
   to step 3, and that notice is lost, because the episode ends here and no
   later attempt writes it. The clear deadline does not apply to this
   branch. The retries add at most 3 s to each notice.
3. In one execution-lock hold, require `LinkGeneration == G+1` and link state
   `disconnected`, then set link state `cleared` and increment the
   generation. A failed compare means Stop won; the attempt rolls back.
4. Persist and publish the `cleared` link state, then pass the typed error to
   `handleStreamDisconnectWithAttempt`, as a live stream's typed error reaches
   it today. The session takes the outcome that path assigns, with #3598's
   `DURABLE_DELIVERY_UNCERTAIN` marking in the prompt branch. The installed
   client stays, as for any failed execution, so that path can stop the
   instance.

### Rollback after commit

After step 3 has installed a new client, a failed attempt undoes the install
so that no later attempt finds a half-live transport:

1. In one execution-lock hold: if the execution's client is still the one
   this attempt installed, remove it. Then, if `LinkGeneration == G+1`, set
   `LinkGeneration = G+2`, keep link state `disconnected`, and set
   `last_error`. If the generation differs, Stop owns the link state and the
   link fields are left as they are.
2. Outside the lock: stop the new stream if step 5 started it, and close the
   client. An unconfirmed stream's close leaves agentctl's budget running. A
   stream whose step 7.2 confirm reached agentctl was attached, so its close
   starts a new agentctl episode with a full budget. That happens in exactly
   these cases, all after the step 7.2 confirm was sent:
   - Stop won at step 7.3. The cleanup then stops the agent.
   - The confirm's response was lost, as a transport error. agentctl may or
     may not have confirmed.
   - The clear deadline passed between step 7.2 and the step 7.3 lock hold.
   - The attempt stream ended between step 7.2 and step 7.3, so `ended` was
     true at step 7.3.

   A repeated confirm and rollback can therefore restart the budget more
   than once in one backend episode. `AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.4`
   requires this: the backend did confirm the stream. It stays within
   `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-004`, because each restart needs
   an attempt that completed steps 1 to 6 on a working link. By then replay
   has shown everything the agent did in the chat. Stop is available the
   whole time. Attempts that the backoff timer starts are at least one
   backoff delay apart; any other attempt needs a reachability change, a
   user Reconnect, or a Kubernetes status tick. When the
   confirm returned 204 and step 1 keeps link state `disconnected`, the
   same lock hold sets the episode's `BudgetDeadline` to the rollback time
   plus the budget, and keeps `Since`.
   After a lost confirm response the deadline is left as it was. It is the
   earlier estimate, and it is correct if the confirm never arrived.
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
reached its step 3 or step 7.3 guard fails that guard. An attempt past step 3
rolls back, and its rollback leaves the link state to Stop. Then:

- The existing stop path moves the session to its stopped state. The
  execution stays tracked in the lifecycle manager, because its agent may
  still run on the host.
- The user Reconnect trigger is removed. The backoff, reachability, and
  Kubernetes triggers stay, and serve only the cleanup. The step counter
  continues from its current value. The same lock hold replaces the pending
  backoff timer with one that captures the new generation (see [Backoff and
  timer rules](#backoff-and-timer-rules)), so the cleanup always has a
  scheduled attempt. With an attempt in flight, that attempt's end schedules
  it.
- Any pending prompt-completion waiter resolves as cancelled, the same way a
  stop resolves it today.

A cleanup attempt runs step 1 of the coordinator, then:

1. If the redial returns a refresh, run these calls in order on the new
   client. All three are HTTP requests, so no event stream is connected, no
   replay runs, and no admission or intake happens. `Client.Cancel` is not
   used, because it needs a connected agent stream.
   1. If the execution has a submission ID, `CancelDeliverySubmission`. It
      marks the journal record cancelled, so nothing can resend the prompt.
      A record already `cancelled`, `completed`, or `failed` returns success
      today, and a 404 counts as done.
   2. `Client.Stop` (`POST /api/v1/stop`). It stops the agent process group,
      which ends the running turn. A manager already stopped or stopping
      returns success today.
   3. The executor's `StopInstance`. An instance that no longer exists counts
      as done.

   The detached output stays in the journal and is removed with the
   instance.
2. If the redial returns `ErrRedialTargetGone`, nothing remains to stop.
3. After either, stop tracking the execution and write link state
   `cleared` with no pending stop. The session stays stopped.
   Reconciliation does not change a stopped session's outcome.
4. On `ErrRedialOrphanUnreaped` or any other error in 1, keep
   `stopped_pending_cleanup` with `last_error` and retry on the next trigger.
   A retry repeats all of 1 from the start. Every call treats "already done"
   as success, so a retry after a lost response converges instead of failing
   forever.

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
   `SetSessionAgentLinkIfNewer`. A failed sweep is retried up to 3 times,
   1 s apart. If it still fails, the failure is logged and counted, and
   startup continues. A stale link is then repaired on first use, as the
   end of this section states.
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

A stored link can still read `disconnected` or `stopped_pending_cleanup`
after a failed sweep. So before it returns `ErrAgentLinkNotDisconnected`, the
manager checks the session's stored `agent_link`. When no execution is
tracked for the session and the stored state is `disconnected` or
`stopped_pending_cleanup`, it writes `cleared` through
`SetSessionAgentLinkIfNewer` with the stored `link_revision` plus one. The
reload then reads `cleared`, and the banner goes. Stop on such a session runs
the existing stop path, and the same repair runs first. A write error is
returned with the typed error, and the next Reconnect or Stop repeats the
repair. The seed in step 2 reads the same stored value, so an execution
created later still writes newer revisions.

## Related decisions

- [ADR-2026-09-27-backend-dialed-detached-agents](../../../decisions/2026-09-27-backend-dialed-detached-agents.md)
- [Durable sessions across harness generations](../../../decisions/2026-09-10-durable-harness-session-boundaries.md)
