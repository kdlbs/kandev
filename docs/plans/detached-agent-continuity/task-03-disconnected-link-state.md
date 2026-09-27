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
  - ../../specs/platform/system-design/detached-agent-continuity-01.md
---

# Task 03: Disconnected link state

## Summary

For a redial-capable executor, a stream disconnect enters a Disconnected link
state instead of failing the execution. The turn stays open, the state is
persisted and published, and Stop is recorded until a reconnect can deliver
it.

## In scope

- **Disconnect branch:** a branch in `handleStreamDisconnectWithAttempt`
  (`lifecycle/manager_events.go`), placed before both the prompt and the idle
  failure branches. The conditions are those in the design section
  "Disconnected link state", including the classification table, the
  capability gate, and the link generation check.
- **Capability gate:** read `GET /identity` after the health check at launch
  and adoption; store `DetachedContinuity` on the execution.
- **Execution state:** `LinkState`, `LinkGeneration`, and `LinkRevision` on
  `AgentExecution`, under the execution lock, and the episode record
  (`EpisodeID`, `EpisodeStartSequence`, recorded events) started on entry.
  Stop increments the generation in the same lock hold that sets
  `stopped_pending_cleanup`. In the prompt branch the
  prompt-completion waiter stays pending and no uncertain code is set. In
  the idle branch nothing waits.
- **Prompts while Disconnected:** a prompt send returns the typed
  `ErrAgentLinkDisconnected`. Queued messages stay queued, including a queue
  dispatch the orchestrator tries on a replayed turn end.
- **Event:** the new `PublishAgentLinkEvent` with `AgentLinkPayload`,
  published as `events.AgentctlDisconnected` (`agentctl.disconnected`) and
  mapped to WS `session.agentctl_disconnected`.
- **Persistence:** the injected `AgentLinkWriter` and the repository method
  `SetSessionAgentLinkIfNewer`, a single-key update guarded by
  `link_revision`. Order: memory, then SQL, then event. A write failure is
  logged and counted and the event still publishes.
- **Stop:**
  - Stop while disconnected marks the session stopped and sets link state
    `stopped_pending_cleanup` with `pending_stop`;
  - the user Reconnect trigger is removed;
  - admission stays blocked;
  - the cleanup is executed by task 04.
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
3. Stop on a Disconnected session records `stopped_pending_cleanup` and
   blocks admission. A non-redial executor, an agentctl without
   `detached-continuity.v1`, and a disconnect classified `terminal` keep the
   current failure path.
4. An idle stream loss enters Disconnected, and a prompt send then returns
   `ErrAgentLinkDisconnected`.
5. A disconnect callback from an older link generation changes nothing. An
   `agent_link` write with an older revision is not stored. Each row of the
   classification table has a test, including an `errors.Join` of a
   transport error and a #3598 typed error.

## Verification

```bash
(cd apps/backend && go test -race -count=1 ./internal/agent/runtime/lifecycle/... -run 'TestStreamDisconnectEntersDisconnected|TestIdleDisconnectEntersDisconnected|TestDisconnectClassification|TestStaleDisconnectCallbackIgnored|TestCapabilityGate|TestDisconnectedHasNoTimerExit|TestStopWhileDisconnected|TestNonRedialExecutorStillFails')
(cd apps/backend && go test -race -count=1 ./internal/orchestrator/... -run 'TestAgentLinkMetadataPersisted')
(cd apps/backend && go test -race -count=1 ./internal/task/repository/sqlite/... -run 'TestSetSessionAgentLinkIfNewer')
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
- `apps/backend/internal/agent/runtime/lifecycle/events.go`:
  `PublishAgentLinkEvent`
- `apps/backend/internal/task/repository/sqlite/session.go`:
  `SetSessionAgentLinkIfNewer`
- `apps/backend/internal/orchestrator/`: the stop path and the prompt
  rejection handling

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
