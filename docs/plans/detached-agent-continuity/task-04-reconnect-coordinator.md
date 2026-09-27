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
    attempts;
  - a `session.reconnect` WS action, authorized like `RetrySessionDelivery`.
- **Attempt steps,** each guarded by the episode generation: redial; pending
  stop cleanup if the link state is `stopped_pending_cleanup`; commit;
  `GetDeliveryStatus` and synchronous `ReplayRecoveredDelivery` to its
  high-water barrier; the new synchronous `StreamManager.ConnectFromCursor`;
  `reconcileDisconnectedSubmission`; reconnected notice; clear under the
  lock and publish `agentctl.ready` with `reconnected_after_ms`. Never
  `StreamManager.ReconnectAll`. The failure table in the design section
  "Reconnect coordinator" applies.
- **Pending stop cleanup:** on a refresh, run `agent.cancel` and stop the
  instance with no stream and no replay; on target gone, nothing to stop;
  then write `cleared`. On an error, keep `stopped_pending_cleanup` and
  retry on the next trigger.
- **Orphan reap failure:** `ErrRedialOrphanUnreaped` keeps the session
  Disconnected with `orphan_reap_failed` and reports no outcome.
- **Orchestrator notices** through `CreateSessionMessageIdempotent`, with
  the name-based message IDs from the design section "Notices":
  - reconnected after a duration, keyed by session and episode generation,
    written before the clear;
  - turn ended while disconnected, keyed by the terminal event's stream and
    sequence (the workflow effect still runs once through
    `processOnTurnCompleteViaEngineWithCause`);
  - paused by the offline budget, keyed by the replayed
    `agent_link.offline_budget_exhausted` event's stream and sequence. No
    queued prompt is dispatched automatically after it.
- **Metrics:** the metrics listed in the system design.

## Out of scope

- The SSH, remote Docker, and Sprites redial bodies (tasks 05-07). Test with
  a fake redialer.
- UI (task 08).

## Acceptance

1. A reachability-returned event starts an attempt within 10 s. Backoff
   follows the schedule. User Reconnect runs immediately and cancels the
   pending timer.
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

## Verification

```bash
(cd apps/backend && go test -race -count=1 ./internal/agent/runtime/lifecycle/... -run 'TestReconnect|TestUserReconnect|TestKubernetesRedialAdapter|TestRedialIdentity|TestStaleAttemptIgnored|TestPendingStopCleanup')
(cd apps/backend && go test -race -count=1 ./internal/orchestrator/... -run 'TestReconnectNotices|TestReconnectNoticesIdempotent|TestTurnEndedWhileDisconnected|TestBudgetPauseNoAutoDispatch')
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
  call sites only
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

- System design sections: Redial contract; Reconnect coordinator; Stop while
  disconnected; Link events and notices; Observability.

## Results

Pending.
