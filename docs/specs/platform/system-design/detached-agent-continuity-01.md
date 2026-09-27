---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-001
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-002
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-006
---

# Detached Agent Continuity System Design Part 1

## Purpose and boundaries

This design keeps a remote agent useful while its backend link is down. Part 1
covers the backend side: how the backend tracks a Disconnected execution, how
it redials the executor transport, and what the user sees.
[Part 2](detached-agent-continuity-02.md) covers agentctl and the executor
host while no backend is attached: the orphan reap, Kandev tool calls, the
offline budget, and agent guidance.
[Part 3](detached-agent-continuity-03.md) covers how the backend leaves
Disconnected: reconnect attempts, rollback, Stop while Disconnected, and a
backend restart.

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
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-001` | [Disconnected link state](#disconnected-link-state); part 3: [Stop while disconnected](detached-agent-continuity-03.md#stop-while-disconnected) |
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-002` | [Redial contract](#redial-contract); part 3: [Reconnect coordinator](detached-agent-continuity-03.md#reconnect-coordinator) |
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
(`lifecycle/manager_events.go`), which today sets `v1.AgentStatusFailed` in
both of its branches: the prompt branch (`promptGeneration != 0`, with the
durable delivery uncertain marking) and the idle branch
(`promptGeneration == 0`).

A new check runs before both failure branches. The disconnect enters
Disconnected when all of these hold:

- the execution's executor implements `RemoteTransportRedialer`;
- the execution is detached-continuity capable (see
  [Capability gate](#capability-gate));
- the disconnect classifies as `transport_loss` or `auth_rejected` in the
  [classification table](#disconnect-classification);
- the execution is not being stopped;
- the disconnect callback's link generation equals the execution's current
  link generation (see [Link generation](#link-generation)).

Otherwise the current path runs unchanged, including #3598's
`DURABLE_DELIVERY_UNCERTAIN` marking. The startup path
(`handleStreamDisconnectWithStartupGeneration`) never enters Disconnected,
because no stream has been established and capability is not yet known.

Entering Disconnected does these things, in this order, under the execution
lock:

1. It increments the link generation and link revision, and records
   `LinkState{State: disconnected, Generation, Revision, Since, Host,
   BudgetDeadline}` on the execution. `BudgetDeadline` is `Since` plus the
   resolved offline budget. It is an estimate; see
   [part 2](detached-agent-continuity-02.md#displayed-pause-time) for the
   authoritative clock. It starts a new episode record: `EpisodeID`, equal
   to the new generation; `EpisodeStartSequence`, equal to the execution's
   projected cursor; and an empty list of recorded events (see
   [Notices](#notices)).
2. It keeps the execution tracked and leaves its status `Running`. It does
   not set a failure code.
3. In the prompt branch, it keeps the prompt-completion waiter pending, so
   the running turn is not completed or failed. The waiter resolves when
   replay delivers the turn's terminal event, or through durable delivery
   reconciliation if the agent is gone. An in-flight submission is not
   marked uncertain here. The reconnect's
   [submission classification](detached-agent-continuity-03.md#submission-classification)
   decides it.
4. In the idle branch, no waiter exists. The execution stays tracked with no
   turn.

After the lock is released, it persists and publishes the link state (see
[Persistence](#persistence)) and starts the
[reconnect coordinator](detached-agent-continuity-03.md#reconnect-coordinator).

The task session state stays `RUNNING`, which is true on the executor host.
Disconnected is a link attribute, not a new `TaskSessionState`. Every existing
state consumer, filter, and workflow guard therefore stays unchanged. No timer
leaves Disconnected. The only exits are the three that
`AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.3` names.

### Prompts while Disconnected

`Manager` rejects a prompt send to a Disconnected execution with the typed
`ErrAgentLinkDisconnected` before it touches the client. The orchestrator
treats that error like a busy session. A message the user queues stays in the
existing session message queue and is not dispatched while the link state is
anything other than connected. A queue dispatch that the orchestrator tries in
that time, for example on a turn end that replay delivers, gets
`ErrAgentLinkDisconnected` and leaves the message queued. After a reconnect
clears the link state, the queue follows its existing rules, except after a
budget pause (see [Queue after a clear](#queue-after-a-clear)). The chat input is
disabled while Disconnected.

### Capability gate

agentctl advertises a new capability, `detached-continuity.v1`, in
`SurvivalCapabilities` (`agentctl/server/api/identity.go`). A build that
advertises it implements the offline budget, the waiting Kandev client, and
the `agent.pgid` record. It advertises it only alongside `agent-delivery.v1`.

The lifecycle manager reads `GET /identity` once after the agentctl health
check at launch and at adoption, and stores
`DetachedContinuity bool` on the execution. The value is true only when the
capability list contains `detached-continuity.v1` and the execution has a
durable delivery descriptor. A fetch error or a missing capability stores
false.

An execution whose agentctl lacks the capability keeps the current failure
path, even on a redial-capable executor. This covers an agentctl that was
started before the upgrade and survived it.

### Disconnect classification

The stream's disconnect error is classified by the first row that matches.
For an `errors.Join` value, each row tests every member with `errors.Is` or
`errors.As`, so the earliest matching row wins regardless of member order.

| Row | Condition | Class | Behavior |
| --- | --- | --- | --- |
| 1 | Expected workspace rebind (`isExpectedWorkspaceRebindDisconnect`) | `ignored` | Unchanged: ignored |
| 2 | Manager shutting down | `ignored` | Unchanged: debug log |
| 3 | Execution stop in progress | `stopping` | Unchanged: stop path |
| 4 | #3598 typed delivery error: cursor behind retention, stream identity mismatch, sequence error, journal error | `delivery` | Unchanged: durable delivery owns it |
| 5 | Stream overload (`agentctl.ErrAgentEventQueueFull`, `errStreamEventProcessorQueueFull`) | `overload` | Unchanged: `shouldReconnectAfterStreamOverload`; if that reconnect fails, its error is classified again from row 1 |
| 6 | agentctl closed the stream deliberately: WebSocket close code 1000 with reason `agent_exited` or `agentctl_shutdown` | `terminal` | Unchanged: failure path |
| 7 | Decode or protocol error on a frame | `terminal` | Unchanged: failure path |
| 8 | HTTP 401 or 403 on the stream or its health check | `auth_rejected` | Disconnected, with `last_error` set to `auth` |
| 9 | `io.EOF`, `io.ErrUnexpectedEOF`, `net.Error` (including timeouts), connection reset or refused, WebSocket close 1006, `ErrSSHTransportLost`, port-forward closed | `transport_loss` | Disconnected |
| 10 | Anything else | `terminal` | Unchanged: failure path |

Row 6 is new on the agentctl side. agentctl sends that close frame before it
ends the stream because the agent process exited or agentctl is shutting
down. Row 10 keeps unknown causes on the existing path, so a new error type
cannot silently hold a session open.

### Link generation

`AgentExecution` gains two counters, guarded by the execution lock:

- `LinkGeneration` increments on entering Disconnected, on Stop while
  Disconnected, when an attempt commits a new client, when an attempt rolls
  back after that commit, when a typed replay error ends the episode, and on
  cleanup. An attempt in flight when the user
  stops is therefore stale, and cannot connect a stopped session. The
  episode's `EpisodeID` does not change within an episode.
- `LinkRevision` increments on every link state change, including attempt
  results inside one episode.

Rules:

- Each stream connect captures the current `LinkGeneration` in its
  disconnect callback. A callback whose generation is not current is dropped
  with a debug log. A stale stream therefore cannot re-enter Disconnected
  after a reattach.
- Each reconnect attempt, backoff timer, and user trigger captures the
  generation it serves. Its result applies only if the generation is still
  current. The compare runs inside the same execution-lock hold as the write
  it guards, and Stop takes the same lock. On a failed compare the attempt
  aborts or rolls back (see [part
  3](detached-agent-continuity-03.md#attempt-steps)).
- A disconnect callback from an attempt's own stream, before that attempt
  clears, fails the attempt instead of entering Disconnected again.
- Every persisted `agent_link` value and every link event carries
  `link_generation` and `link_revision`. The store and the frontend apply a
  value only when its revision is newer than the one they hold.

## Redial contract

The executor contract generalizes the Kubernetes refresher:

```go
// RedialIdentity is the durable delivery identity that the redialed agentctl
// must still own.
type RedialIdentity struct {
    StreamID          string
    SessionID         string
    IncarnationID     string
    HarnessGeneration uint64
}

// RemoteTransportRedialer re-establishes the transport to the agentctl that
// already runs instance, without starting a new one.
type RemoteTransportRedialer interface {
    RedialRemoteInstance(ctx context.Context, instance *ExecutorInstance, expect RedialIdentity) (*RemoteInstanceRefresh, error)
    // DropRedialedTransport tears down a committed redial transport after a
    // failed attempt and marks it lost. It is idempotent.
    DropRedialedTransport(instance *ExecutorInstance)
}
```

It returns the existing `RemoteInstanceRefresh` (`lifecycle/executor_backend.go`),
so the manager keeps a single staged-swap path. `Commit(publish)` installs the
new agentctl client under the execution write lock, after a link generation
guard in the same lock hold (see [part
3](detached-agent-continuity-03.md#attempt-steps)). `Abort` releases a
half-built transport. `ProcessRestarted` is always false on a redial result.
Any evidence that agentctl was replaced returns `ErrRedialTargetGone`
instead.

### Identity check

Every executor calls one shared helper, `verifyRedialIdentity`, on the staged
client before it returns a refresh:

1. The agentctl health check passes with the stored auth token.
2. `GET /identity` lists `detached-continuity.v1`.
3. `GetDeliveryStatus(ctx, expect.StreamID)` returns a descriptor whose
   `SessionID`, `IncarnationID`, `HarnessGeneration`, and `StreamID` equal
   `expect`. The coordinator builds `expect` from the execution's
   `DeliveryDescriptor`.

| Check result | Redial result |
| --- | --- |
| All match | refresh |
| Transport error during any check | `ErrRedialUnreachable` |
| HTTP 401 or 403 | other error (`auth`) |
| Capability missing, stream unknown, or any identity field differs | [Reap the orphan](detached-agent-continuity-02.md#orphaned-agent-after-agentctl-loss), then `ErrRedialTargetGone` |

A matching PID is not identity. The SSH PID probe is only a fast path to
`ErrRedialTargetGone`.

### Results

| Result | Meaning | Coordinator action |
| --- | --- | --- |
| refresh | Transport re-established; the identity check passed | Continue the attempt |
| `ErrRedialUnreachable` | Host or transport cannot be reached | Stay Disconnected and schedule the next attempt |
| `ErrRedialTargetGone` | Host reached; the recorded agentctl no longer exists; any orphan was reaped or already gone | Leave Disconnected through reconciliation |
| `ErrRedialOrphanUnreaped` | Host reached; the agentctl is gone; its agent group could not be stopped | Stay Disconnected with a blocking `last_error`; report no outcome; retry on the next trigger |
| other error | Authentication, host-key, or configuration failure | Stay Disconnected, publish the error in the link state, back off at the cap |

### Per executor

- **SSH** (`executor_ssh.go`):
  - Redial takes the lost `sshSessionState` out of `SSHExecutor.sessions`.
    That entry currently makes `ResumeRemoteInstance` return early and
    `CreateInstance` return `ErrSSHTransportLost`.
  - It dials with the recorded target and pinned host-key fingerprint.
  - It probes the recorded agentctl `pid`. A dead pid goes to the orphan
    reap, then `ErrRedialTargetGone`.
  - It opens a new forward with `StartPortForward`, which gives a new local
    port, then runs the identity check.
  - It installs a fresh `sshSessionState`, with a new keepalive watchdog,
    under the same InstanceID.
  - The swap happens under `r.mu`, so a concurrent `CreateInstance` sees
    either the lost entry or the new one, never a gap.
- **Remote Docker** (`executor_remote_docker.go`):
  - The watchdog's `takeSession` deletes the session. The executor therefore
    keeps the dial target in `targets` when the loss is a transport loss.
  - Redial rebuilds the SSH client, the Docker client, and the forward from
    that target through the existing `reconnect` hook and
    `reconnectToContainer`, then runs the identity check.
  - A missing container yields `ErrRedialTargetGone`. Nothing is left to
    reap, because the container's processes ended with it.
- **Sprites** (`executor_sprites*.go`): redial resolves the sprite with
  `reconnectSprite`, opens a new proxy with `setupPortForwarding`, replaces
  the entry in `proxies`, and runs the identity check. A sprite that no
  longer exists yields `ErrRedialTargetGone` with nothing to reap. The
  in-sprite orphan reap runs through the sprite command API and belongs to
  task 07, which is blocked.
- **Kubernetes** (`executor_kubernetes_refresh.go`): `RedialRemoteInstance`
  calls the existing `RefreshRemoteInstance`, then maps its result:
  - `ProcessRestarted == false`: run the identity check on the refresh's
    client and return the refresh.
  - `ProcessRestarted == true`: call `Abort` and return `ErrRedialTargetGone`.
    The redial path never reaches `prepareRestartedKubernetesAgentctl`, so it
    never creates an ACP session. No reap is needed, because the container
    restart ended every process in the container.

  The 60-second `remoteStatusLoop` keeps its current behavior, including the
  restart path, for an execution whose link state is connected. For an
  execution in any other link state, the loop does not refresh. It triggers
  a coordinator attempt with trigger `k8s_refresh` instead.

## Reconnect coordinator

[Part 3](detached-agent-continuity-03.md) defines how the backend leaves
Disconnected: the coordinator and its backoff and timer rules, the attempt
steps from redial to clear, submission classification, rollback after a
failed attempt, Stop while Disconnected, and a backend restart.

## Link events and notices

### Link events

Link state changes publish through a new publisher method with a typed
payload. The existing `PublishAgentctlEvent(ctx, eventType, execution,
errMsg)` (`lifecycle/events.go`) carries only an error string and does not
change.

```go
type AgentLinkPayload struct {
    State              string     // connected, disconnected, stopped_pending_cleanup, cleared
    LinkGeneration     uint64
    LinkRevision       uint64
    Since              time.Time
    Host               string
    BudgetDeadline     time.Time  // estimate, see Displayed pause time
    NextAttemptAt      *time.Time
    LastError          string     // closed set: auth, host_key, unreachable, orphan_reap_failed, replay, config
    PendingStop        bool
    ReconnectedAfterMS int64      // set on the change to connected
    BudgetPaused       bool       // set on the change to connected
}

PublishAgentLinkEvent(ctx context.Context, execution *AgentExecution, payload AgentLinkPayload)
```

| Change | Constant | WS action |
| --- | --- | --- |
| Enter Disconnected, attempt result, stop pending cleanup | `events.AgentctlDisconnected` (`agentctl.disconnected`) | `session.agentctl_disconnected` |
| Reconnected or cleared | existing `events.AgentctlReady` | `session.agentctl_ready` |

The gateway maps both in `gateway/websocket/task_notifications.go` and passes
the payload through.

### Write order

The lifecycle manager is the only writer of link state. Each change runs in
this order:

1. **Memory.** Update `LinkState` under the execution lock and take the new
   revision. Memory is authoritative while the backend runs.
2. **SQL.** Call the injected `AgentLinkWriter`, wired like the existing
   `runningWriter`. See [Persistence](#persistence).
3. **Event.** Publish `PublishAgentLinkEvent` with the same payload.

A write failure in step 2 is logged and counted, and step 3 still runs. Each
write stores a full snapshot, so the next successful write repairs a missed
one. The orchestrator's stop path does not write `agent_link`. It asks the
lifecycle manager to stop, and the manager records `pending_stop` through
the same three steps.

### Notices

The orchestrator writes conversation notices as status messages through
`CreateSessionMessageIdempotent`. As `persistRecoveryStatusMessage` does, each
message ID is a name-based UUID (`uuid.NewSHA1(uuid.NameSpaceOID, key)`) of a
deterministic key. A retry, a replay, or a second attempt in the same episode
writes the same ID, so the notice appears at most once.

An episode is bounded by journal sequence, not by time. It covers the
sequences above its `EpisodeStartSequence` and at or below the
`AttachedAtSequence` that [step 6](detached-agent-continuity-03.md#attempt-steps)
reads, which is the journal high water when the attempt's stream became
current. After a [typed replay error](detached-agent-continuity-03.md#typed-replay-error),
the projected cursor is the upper bound instead. While
the link state is not connected, inbox projection records on the episode each
terminal turn event and each `agent_link.offline_budget_exhausted` event
above `EpisodeStartSequence`, with its sequence and time, once per sequence.
The events can arrive by replay or by the live stream, and both are
recorded. A rollback keeps the record.

| Notice | Key | Written when |
| --- | --- | --- |
| Turn ended while disconnected | `agent-link-turn-ended:<stream_id>:<sequence>` of the terminal event | Step 7.1, for each recorded terminal event at or below `AttachedAtSequence`, with the event's `created_at` |
| Paused by the offline budget | `agent-link-budget:<stream_id>:<sequence>` of the budget event | Step 7.1, for a recorded budget event at or below `AttachedAtSequence` whose `outcome` is `cancelled` or `stopped`, with its journaled `exhausted_at` |
| Reconnected after a duration | `agent-link-reconnected:<session_id>:<episode_id>` | Step 7.5, after the guard in step 7.3 held and the link state cleared |

Ordering rules:

- Step 7.1 writes the turn-ended and budget notices in ascending journal
  sequence of their events. Sequence is the tiebreak for equal times. The
  reconnected notice comes after all of them.
- A failure between step 7.1 and the clear leaves the session Disconnected.
  The next attempt in the same episode writes the same IDs, so nothing
  duplicates. If Stop wins the guard, the notices already written stay,
  because each records a journaled fact, and no reconnected notice is
  written.
- One agentctl episode records at most one budget event (see [part
  2](detached-agent-continuity-02.md#offline-budget)). If an episode still
  records more than one, only the lowest sequence gets a notice, and a
  warning is logged.
- A recorded event above `AttachedAtSequence` happened after the stream
  became current. It gets no notice. A budget event can never be above it,
  because a stream start
  waits until budget enforcement has ended (see [part
  2](detached-agent-continuity-02.md#budget-enforcement)).
- A budget event with outcome `stop_failed` gets no notice and sets no
  budget flag, because its turn still runs. It is logged and counted.
- The turn-ended notice sits beside the workflow effect and gates nothing.
  The workflow effect still runs once, through `workflowEffectForTurn` and
  `processOnTurnCompleteViaEngineWithCause`, with its own durable delivery
  effect key.

### Queue after a clear

The episode's budget flag is true when it recorded a budget event at or
below `AttachedAtSequence` with outcome `cancelled` or `stopped`. Step 7.3 reads it in the same lock hold that sets
link state `connected`, and publishes it as `BudgetPaused`. All detached
events are projected by then, so the flag cannot change after the clear.

On the `AgentctlReady` event of a clear, the orchestrator:

- with `BudgetPaused` false: runs its existing queue dispatch for the session
  once, as it would after a turn end;
- with `BudgetPaused` true: dispatches nothing and marks the session's queue
  held. The hold ends when the user sends a new message, or uses the queue's
  existing Send now action (`message.queue.send_now`) on a queued message.
  Then the queue follows its existing rules. The hold lives in memory, and a
  backend restart ends it.

## Frontend

- `SessionAgentctlStatus.status` (`lib/state/slices/session/types.ts`) gains
  `"disconnected"` and `"stopped_pending_cleanup"`, with `since`, `host`,
  `budgetDeadline`, `nextAttemptAt`, `lastError`, `linkGeneration`, and
  `linkRevision`.
- `lib/ws/handlers/agent-session.ts` handles `session.agentctl_disconnected`
  and the link payload on `session.agentctl_ready`. It drops a payload whose
  `linkRevision` is not newer than the stored one. On load, the session's
  `agent_link` metadata seeds the same status, which keeps state across a
  reload (`AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.6`).
- Banner selection in `chat-input-container.tsx`:

  | Link state | Session state | Banner |
  | --- | --- | --- |
  | `disconnected` | `RUNNING` | `DisconnectedSessionBanner` |
  | `stopped_pending_cleanup` | stopped | existing `SessionStoppedBanner`, plus a cleanup line |
  | `connected`, `cleared`, or none | any | existing behavior |

  The shared `ChatInputArea` renders both on phones too.
- **`DisconnectedSessionBanner` actions:**
  - Reconnect sends `session.reconnect`.
  - Stop uses the existing stop action.
  - Test IDs are `disconnected-session-banner`,
    `disconnected-reconnect-button`, and `disconnected-stop-button`.
- The stopped banner's cleanup line has no Reconnect action. Its test ID is
  `stopped-pending-cleanup-line`.
- The pause time is shown as approximate, because it is the backend's
  estimate (see [part 2](detached-agent-continuity-02.md#displayed-pause-time)).
- `RemoteCloudTooltip` and the kanban status icon show the disconnected
  state from the same status.
- Copy uses `task:` keys in all six locales, with no em dash:
  - `agentDisconnectedTitle`
  - `agentDisconnectedBody`
  - `agentDisconnectedPauseAt`, worded as an approximate time
  - `agentReconnect`
  - `agentReconnectedAfter`
  - `agentTurnEndedWhileDisconnected`
  - `agentPausedByOfflineBudget`
  - `agentStopPendingCleanup`
  - `agentLinkErrorAuth`, `agentLinkErrorHostKey`,
    `agentLinkErrorOrphanReap`, for `lastError`

## Failure and recovery

| Condition | Behavior |
| --- | --- |
| Host unreachable for hours | Disconnected; backoff at 5 min; budget cancels the turn at its deadline; reconnect later replays the cancellation |
| Host reachable, agentctl gone (host reboot, OOM) | Reap the orphaned agent group, then `ErrRedialTargetGone`; durable delivery reconciliation decides the outcome |
| Host reachable, agentctl gone, reap fails | `ErrRedialOrphanUnreaped`; stay Disconnected with `orphan_reap_failed`; no outcome; the next trigger retries |
| Host reachable, a different agentctl answers | Identity check fails; reap, then `ErrRedialTargetGone` |
| Kubernetes container restarted | `ErrRedialTargetGone` with no reap; no new ACP session |
| Host-key mismatch or authentication failure | Stay Disconnected, show `last_error`, back off at the cap; never re-pin a host key automatically |
| Redial succeeds, stream replay fails (cursor invalid, journal lost) | Link state `cleared`; durable delivery's typed error path assigns the outcome; see [part 3](detached-agent-continuity-03.md#typed-replay-error) |
| Redial succeeds, transport drops during replay | Close the new client; stay Disconnected; next attempt on backoff |
| Stale disconnect callback or attempt result | Dropped by the link generation check |
| agentctl predates the capability | Current failure path |
| Backend restarts while Disconnected or stopped pending cleanup | The startup sweep clears `agent_link`, and the existing restart path applies; see [part 3](detached-agent-continuity-03.md#backend-restart) |
| Attempt fails after its commit | Rollback removes the client and drops the transport; see [part 3](detached-agent-continuity-03.md#rollback-after-commit) |
| Two triggers fire together | Single-flight per execution; the second joins the first attempt and does not advance the backoff |

## Persistence

`task_sessions.metadata.agent_link` stores the `AgentLinkPayload` fields
except `NextAttemptAt` and `ReconnectedAfterMS`:

- `state`
- `link_generation`
- `link_revision`
- `since`
- `host`
- `budget_deadline`
- `pending_stop`
- `last_error`

`AgentLinkWriter` is implemented by the task repository with a new method,
`SetSessionAgentLinkIfNewer(ctx, sessionID, value) (stored bool, err error)`.
It updates only the `agent_link` key, in one statement, like
`SetSessionMetadataKey`, so writes to other metadata keys are not lost. The
statement stores the value only when the stored `link_revision` is absent or
lower, so an out-of-order write cannot replace a newer one.
`stored == false` is not an error.

The manager writes `agent_link` on each change. On reconnect it writes state
`connected`. On a terminal outcome or after stop cleanup, it writes `cleared`.
The key is never deleted, so the revision guard keeps working. The startup
sweep and the counter seeding in [part
3](detached-agent-continuity-03.md#backend-restart) keep the guard working
across a backend restart. The frontend
treats `connected`, `cleared`, and an absent key the same way. It needs no new
table and no migration.

agentctl writes `agent.pgid` (process group ID and start time) in the session
directory on the executor host, and removes it when it stops the agent. The
agentctl journal (durable delivery) keeps the detached output and the budget
event.

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
| `agent_link_reconnect_attempts_total` | `executor_type`, `trigger` (`reachability`, `backoff`, `user`, `k8s_refresh`), `outcome` (`reconnected`, `unreachable`, `target_gone`, `orphan_unreaped`, `error`, `stale`) |
| `agent_link_disconnected_seconds` (histogram) | `executor_type` |
| `agent_link_offline_budget_exhausted_total` | `executor_type` |
| `agent_link_orphan_reap_total` | `executor_type`, `outcome` (`reaped`, `already_gone`, `reap_failed`) |
| `agent_link_kandev_call_wait_total` | `outcome` (`answered`, `budget_exhausted`, `unknown_outcome`) |
| `agent_link_write_failed_total` | `target` (`state`, `notice`) |
| `agent_link_budget_journal_failed_total` (agentctl) | none |

Each transition also logs one structured zap line with the execution and
session IDs and the link generation.

## Related decisions

- [ADR-2026-09-27-backend-dialed-detached-agents](../../../decisions/2026-09-27-backend-dialed-detached-agents.md)
- [Durable sessions across harness generations](../../../decisions/2026-09-10-durable-harness-session-boundaries.md)
