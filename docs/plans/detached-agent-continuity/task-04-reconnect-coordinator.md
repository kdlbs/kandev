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
  - ../../specs/platform/system-design/detached-agent-continuity.md
---

# Task 04: Reconnect coordinator and redial contract

## Summary

A single-flight coordinator reconnects each Disconnected execution when
executor reachability returns, on a capped backoff, or on user request. It
redials through `RemoteTransportRedialer`, then replays from the durable
cursor and reconciles. It never re-initializes the harness. It also records
the conversation notices.

## In scope

- **Redial contract:** the `RemoteTransportRedialer` contract, with
  `ErrRedialUnreachable` and `ErrRedialTargetGone`.
- **Kubernetes:** an adapter over `RefreshRemoteInstance`.
- **Coordinator in `lifecycle`:**
  - `remoteRefreshGroup` single-flight;
  - a subscriber to `events.ExecutorReachabilityChanged`;
  - a backoff of 5 s doubling to a 300 s cap, with ±20% jitter and unlimited
    attempts;
  - a `session.reconnect` WS action, authorized like `RetrySessionDelivery`.
- **Attempt steps:** redial, then commit, then
  `StreamManager.ReconnectAll`, then `reconcileDisconnectedSubmission`, then
  clear the link state and publish `agentctl.ready` with
  `reconnected_after_ms`.
- **Pending stop:** on reconnect, run `agent.cancel` and stop the instance
  before intake.
- **Orchestrator notices:**
  - reconnected after a duration;
  - turn ended while disconnected (the workflow effect still runs once
    through `processOnTurnCompleteViaEngineWithCause`);
  - paused by the offline budget, from the replayed
    `agent_link.offline_budget_exhausted` event. No queued prompt is
    dispatched automatically after it.
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
   reconciliation. Pending stop is executed on reconnect.

## Verification

```bash
(cd apps/backend && go test -race -count=1 ./internal/agent/runtime/lifecycle/... -run 'TestReconnect|TestUserReconnect|TestKubernetesRedialAdapter')
(cd apps/backend && go test -race -count=1 ./internal/orchestrator/... -run 'TestReconnectNotices|TestTurnEndedWhileDisconnected|TestBudgetPauseNoAutoDispatch')
(cd apps/backend && go test -race -count=1 ./internal/executors/reachability/...)
make -C apps/backend lint
```

## Files likely touched

- New `apps/backend/internal/agent/runtime/lifecycle/reconnect_coordinator.go`
  and its test
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`,
  `executor_kubernetes_refresh.go`, `manager_kubernetes_refresh.go`
- `apps/backend/internal/agent/runtime/lifecycle/streams.go`,
  `durable_delivery_stream.go`: call sites only
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
