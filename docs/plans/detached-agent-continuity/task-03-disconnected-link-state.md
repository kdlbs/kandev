---
id: "03-disconnected-link-state"
title: "Disconnected link state"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-001
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-006
acceptance_criteria:
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.1
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.2
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.5
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.6
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.6
system_design:
  - ../../specs/platform/system-design/detached-agent-continuity.md
---

# Task 03: Disconnected link state

## Summary

For a redial-capable executor, a stream disconnect enters a Disconnected link
state instead of failing the execution. The turn stays open, the state is
persisted and published, and Stop is recorded until a reconnect can deliver
it.

## In scope

- **Disconnect branch:** a branch in `handleStreamDisconnectWithAttempt`
  (`lifecycle/manager_events.go`), placed before the failure path. The
  conditions are those in the design section "Disconnected link state".
- **Execution state:** `LinkState` on `AgentExecution`. The prompt-completion
  waiter stays pending.
- **Event:** `events.AgentctlDisconnected` (`agentctl.disconnected`), mapped
  to WS `session.agentctl_disconnected`. The payload is defined in the system
  design.
- **Persistence:** `task_sessions.metadata.agent_link`, written on enter and
  cleared on exit.
- **Stop:**
  - Stop while disconnected sets `pending_stop` and marks the session
    stopped;
  - admission stays blocked;
  - the cancel-and-stop is executed by task 04 on reconnect.
- **Marker interface:** `RemoteTransportRedialer` is declared here, empty of
  implementations, so the branch compiles. Task 04 adds the contract body and
  implementations.

## Out of scope

- Reconnecting (task 04).
- Executor redials (tasks 05-07).
- UI rendering (task 08).

## Acceptance

1. On a redial-capable execution, a stream loss publishes
   `agentctl.disconnected` within 5 s. It persists `agent_link`, and leaves
   the session `RUNNING` and the task on its step.
2. Advancing a fake clock by hours produces no state change, no step move,
   and no replacement launch.
3. Stop on a Disconnected session records `pending_stop` and blocks
   admission. A non-redial executor keeps the current failure path.

## Verification

```bash
(cd apps/backend && go test -race -count=1 ./internal/agent/runtime/lifecycle/... -run 'TestStreamDisconnectEntersDisconnected|TestDisconnectedHasNoTimerExit|TestStopWhileDisconnected|TestNonRedialExecutorStillFails')
(cd apps/backend && go test -race -count=1 ./internal/orchestrator/... -run 'TestAgentLinkMetadataPersisted')
(cd apps/backend && go test -race -count=1 ./internal/gateway/websocket/...)
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`, and a new
  `manager_events_disconnect_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`: `LinkState`
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`: the
  interface declaration
- `apps/backend/internal/agent/runtime/lifecycle/session.go`: the held
  prompt waiter
- `apps/backend/internal/events/types.go`,
  `apps/backend/internal/gateway/websocket/task_notifications.go`,
  `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/orchestrator/`: the `agent_link` metadata writer and
  the stop path

## Dependencies

None.

## Risks

- **A held waiter can leak.** Every exit must be covered by a test: reconnect
  (task 04), target gone (task 04), user Stop, and backend shutdown.
- **Overlap with #3598.** #3598's `DURABLE_DELIVERY_UNCERTAIN` branch in the
  same function must still run when the executor is not redial-capable.

## Parallelism

`parallel-safe` with task 01.

## Inputs

- System design sections: Disconnected link state; Stop while disconnected;
  Persistence.

## Results

Pending.
