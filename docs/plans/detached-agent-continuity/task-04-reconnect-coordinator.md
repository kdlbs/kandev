---
id: "04-reconnect-coordinator"
title: "Reconnect coordinator and redial contract"
status: pending
wave: 2
depends_on: ["03-disconnected-link-state"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-001
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-002
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-006
acceptance_criteria:
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.3
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.4
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.6
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.1
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.2
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.3
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.4
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.5
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.3
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.4
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.5
system_design:
  - ../../specs/platform/system-design/detached-agent-continuity-01.md
  - ../../specs/platform/system-design/detached-agent-continuity-02.md
  - ../../specs/platform/system-design/detached-agent-continuity-03.md
---

# Task 04: Reconnect coordinator and redial contract

## Summary

A single-flight coordinator reconnects each Disconnected execution when
executor reachability returns, on a capped backoff, or on user request. It
redials through `RemoteTransportRedialer`, then replays from the durable
cursor and reconciles. It never re-initializes the harness. It also records
the conversation notices.

## In scope

- **Redial contract:** the `RemoteTransportRedialer` contract with
  `RedialIdentity`, and `ErrRedialUnreachable`, `ErrRedialTargetGone`, and
  `ErrRedialOrphanUnreaped`. The shared `verifyRedialIdentity` helper
  (health with the stored token, `detached-continuity.v1`, and a
  `GetDeliveryStatus` descriptor matching `RedialIdentity`) that tasks 05-07
  call.
- **Kubernetes:** an adapter over `RefreshRemoteInstance`. `ProcessRestarted`
  maps to `Abort` plus `ErrRedialTargetGone`, so the redial path never
  reaches `prepareRestartedKubernetesAgentctl`. The 60 s loop skips an
  execution not in link state connected and triggers a `k8s_refresh`
  attempt instead.
- **Coordinator in `lifecycle`:**
  - `remoteRefreshGroup` single-flight;
  - a subscriber to `events.ExecutorReachabilityChanged`;
  - a backoff of 5 s doubling to a 300 s cap, with ±20% jitter and unlimited
    attempts, following part 3's "Backoff and timer rules": one step counter
    per episode, advanced by every failed attempt whatever its trigger, reset
    by a new episode, one pending timer, and "other error" at the cap;
  - a `session.reconnect` WS action, authorized like `RetrySessionDelivery`.
- **Attempt steps** from part 3 "Attempt steps", each generation compare
  inside the execution-lock hold of the write it guards: redial; pending
  stop cleanup if the link state is `stopped_pending_cleanup`; guarded
  commit that sets generation `G+1`; `GetDeliveryStatus` and synchronous
  `ReplayRecoveredDelivery` to its high-water barrier; the new synchronous
  `StreamManager.ConnectFromCursor` with a 90 s handshake bound; a second
  `GetDeliveryStatus` for `AttachedAtSequence` and a bounded cursor wait;
  the new `classifyReattachedSubmission` (the existing
  `reconcileDisconnectedSubmission` and its call site stay unchanged);
  notices; guarded clear that publishes `agentctl.ready` with
  `reconnected_after_ms` and `budget_paused`. Never
  `StreamManager.ReconnectAll`. Part 3's failure table applies.
- **Rollback after commit:** remove the installed client, advance the
  generation, close the stream and client, and call the new
  `DropRedialedTransport` (tasks 05-07 implement it per executor; the
  Kubernetes body is here).
- **Backend restart:** the startup `ClearStaleAgentLinks` sweep (SQLite and
  PostgreSQL), counter seeding from the stored `agent_link`, and the typed
  `ErrAgentLinkNotDisconnected` for `session.reconnect`.
- **Pending stop cleanup:** on a refresh, run `agent.cancel` and stop the
  instance with no stream and no replay; on target gone, nothing to stop;
  then write `cleared`. On an error, keep `stopped_pending_cleanup` and
  retry on the next trigger.
- **Orphan reap failure:** `ErrRedialOrphanUnreaped` keeps the session
  Disconnected with `orphan_reap_failed` and reports no outcome.
- **Episode record:** inbox projection records terminal and budget events
  above `EpisodeStartSequence` while the link is not connected, once per
  sequence, kept across rollbacks.
- **Orchestrator notices** through `CreateSessionMessageIdempotent`, with
  the name-based message IDs from part 1 "Notices", all written in step 7
  before the clear, for recorded events at or below `AttachedAtSequence`:
  - turn ended while disconnected, keyed by the terminal event's stream and
    sequence (the workflow effect still runs once through
    `processOnTurnCompleteViaEngineWithCause`);
  - paused by the offline budget, keyed by the budget event's stream and
    sequence, only for outcome `cancelled` or `stopped`;
  - reconnected after a duration, keyed by session and `EpisodeID`, last.
- **Queue after a clear:** with `budget_paused` false, run the existing queue
  dispatch once; with it true, hold the queue until the user sends a message
  or uses `message.queue.send_now`.
- **Metrics:** the metrics listed in the system design.

## Out of scope

- The SSH, remote Docker, and Sprites redial bodies (tasks 05-07). Test with
  a fake redialer.
- UI (task 08).

## Acceptance

1. A reachability-returned event starts an attempt within 10 s. Backoff
   follows the schedule. User Reconnect runs immediately and cancels the
   pending timer. A failed immediate attempt advances the step counter and
   leaves exactly one timer. A new episode restarts at 5 s.
2. A successful attempt never calls `initializeAgentSession` or any launch
   intent. Replayed output appears once and in order, and the notices are
   recorded.
3. `ErrRedialTargetGone` ends Disconnected through durable delivery
   reconciliation. `ErrRedialOrphanUnreaped` reports no outcome and retries
   on the next trigger. Pending stop is executed on reconnect, before any
   stream or replay.
4. The clear waits for replay to reach the barrier. A transport error during
   replay keeps Disconnected. A stale attempt result, from an older episode,
   changes nothing and aborts its staged refresh.
5. A repeated attempt, replay, or notice write in one episode yields each
   notice once. A Kubernetes container restart on the redial path returns
   `ErrRedialTargetGone` and creates no ACP session.
6. Stop racing each guard (commit, clear) never leaves a stopped session
   connected. A failure after commit removes the client and calls
   `DropRedialedTransport`, and the next attempt redials cleanly.
7. Submission classification maps every journal state as part 3's table
   says, never resends, and never double-resolves a waiter that the live
   stream resolved.
8. A terminal or budget event journaled after the step 4 barrier but before
   the attach gets its notice. A budget pause holds the queue; a turn end
   without one dispatches it once.
9. After a restart, the sweep clears `disconnected` and
   `stopped_pending_cleanup` links, the next link write is accepted, and
   `session.reconnect` returns `ErrAgentLinkNotDisconnected`.

## Verification

```bash
(cd apps/backend && go test -race -count=1 ./internal/agent/runtime/lifecycle/... -run 'TestReconnect|TestUserReconnect|TestKubernetesRedialAdapter|TestRedialIdentity|TestStaleAttemptIgnored|TestPendingStopCleanup|TestBackoffTimerRules|TestRollbackAfterCommit|TestStopRacesGuards|TestClassifyReattachedSubmission|TestEpisodeSequenceBounds|TestLinkCounterSeeding')
(cd apps/backend && go test -race -count=1 ./internal/orchestrator/... -run 'TestReconnectNotices|TestReconnectNoticesIdempotent|TestTurnEndedWhileDisconnected|TestBudgetPauseNoAutoDispatch|TestQueueDispatchAfterClear|TestClearStaleAgentLinksOnStartup|TestReconnectNotDisconnected')
(cd apps/backend && go test -race -count=1 ./internal/task/repository/sqlite/... -run 'TestClearStaleAgentLinks')
(cd apps/backend && go test -race -count=1 ./internal/executors/reachability/...)
make -C apps/backend lint
```

## Files likely touched

- New `apps/backend/internal/agent/runtime/lifecycle/reconnect_coordinator.go`
  and its test
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`,
  `executor_kubernetes_refresh.go`, `manager_kubernetes_refresh.go`
- `apps/backend/internal/agent/runtime/lifecycle/streams.go`:
  `ConnectFromCursor`
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream.go`:
  the new `classifyReattachedSubmission`; `reconcileDisconnectedSubmission`
  unchanged
- `apps/backend/internal/task/repository/sqlite/`: `ClearStaleAgentLinks`
- `apps/backend/internal/orchestrator/service.go`: the startup sweep call
- `apps/backend/internal/orchestrator/`: the notices and the
  `session.reconnect` handler
- `apps/backend/pkg/websocket/actions.go`

## Dependencies

- Task 03: link state, the event, and `pending_stop`.
- The notice for budget pause reads task 01's journal event type. Stub it if
  task 01 has not landed.

## Risks

- **Replay can meet the #3598 uncertain block.** Do not bypass
  `checkSessionRecoveryBlock`. Surface the block instead. Its automatic
  resolution is requested of #3598.
- **The Kubernetes 60 s refresh races the coordinator.** Single-flight
  through `remoteRefreshGroup` prevents a double swap.

## Parallelism

`sequential` after task 03. It can run in parallel with task 02.

## Inputs

- System design sections: part 1 Redial contract, Link events and notices,
  Queue after a clear, Observability; part 3 in full.

## Results

Pending.
