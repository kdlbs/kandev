---
id: "01-offline-budget"
title: "Offline budget in agentctl"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-004
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-003
acceptance_criteria:
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.5
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.1
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.2
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.3
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.4
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.5
system_design:
  - ../../specs/platform/system-design/detached-agent-continuity.md
---

# Task 01: Offline budget in agentctl

## Summary

agentctl measures how long no backend stream has been attached. When that
reaches the instance's offline budget and a turn is running, agentctl cancels
the turn and journals the event. The budget defaults to 15 minutes and comes
from the executor profile.

## In scope

- **Detach clock in `agentctl/server/process/attachment.go`:**
  - `MarkDetached` at zero arms the timer;
  - `MarkAttached` disarms it and clears it;
  - `WaitSignals` exposes the `budgetExhausted` channel for task 02.
- **On expiry:**
  - call the adapter's `Cancel`;
  - journal `agent_link.offline_budget_exhausted` with its timestamp;
  - keep agentctl and the journal running.
- **Profile override:**
  - `offline_budget_minutes` in `profileConfigAuthoritativeKeys`
    (`orchestrator/executor/executor_state.go`), validated to 1–1440, with a
    default of 15;
  - carried as `OfflineBudget` through `agentctl.CreateInstanceRequest` and
    `config.InstanceOverrides`.
- **Reaper bound:** `ownershipperiod.Resolve` never resolves below the
  largest active instance budget plus 1 minute.
- **Orphan record:** on agent start, agentctl writes `agent.pgid` (the
  process group ID from `Manager.AgentPID()` and the process start time) in
  the session directory. It removes the file on agent stop. Tasks 05 and 06
  read this file to reap an orphaned agent.

## Out of scope

- Kandev tool call waiting (task 02).
- Backend link state (task 03).
- UI (task 08).

## Acceptance

1. A detached instance with a running turn is cancelled exactly once, at the
   budget. A reattach before the budget cancels nothing, and the clock
   restarts at the next detach.
2. The profile value reaches the instance config. Out-of-range values are
   rejected at launch with a typed error.
3. With agent survival enabled, the unowned reaper cannot stop agentctl
   before the budget has expired. A permission request parked while detached
   stays pending until reattach or until the budget cancels the turn. It is
   never approved or denied automatically.

## Verification

```bash
(cd apps/backend && go test -race -count=1 ./internal/agentctl/server/process/... -run 'TestDetachClock|TestOfflineBudget|TestPermissionParkedUntilBudgetCancel|TestAgentPgidRecord')
(cd apps/backend && go test -race -count=1 ./internal/common/ownershipperiod/... ./internal/orchestrator/executor/... -run 'TestResolveCoversOfflineBudget|TestOfflineBudgetProfileResolution')
(cd apps/backend && go test -race -count=1 ./internal/agentctl/server/config/... ./internal/agent/runtime/agentctl/...)
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/attachment.go`, and a new
  `attachment_test.go`
- `apps/backend/internal/agentctl/server/process/manager.go`: journal event,
  adapter cancel
- `apps/backend/internal/agentctl/server/config/config.go`:
  `InstanceOverrides.OfflineBudget`
- `apps/backend/internal/agent/runtime/agentctl/control.go`:
  `CreateInstanceRequest`
- `apps/backend/internal/orchestrator/executor/executor_state.go` and its test
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`: the
  metadata key constant
- `apps/backend/internal/common/ownershipperiod/period.go` and its test

## Dependencies

None. The journal event type relies on the #3598 journal already present on
the base branch.

## Risks

- The cancel races a turn that ends naturally at the same moment. Treat
  "no turn running" as a no-op.
- The journal is full at expiry. #3598's typed journal error applies. Still
  cancel the turn.

## Parallelism

`parallel-safe` with task 03. The packages are disjoint.

## Inputs

- System design sections: Offline budget; Kandev tool calls while detached,
  for the permission note.
- `docs/specs/platform/system-design/durable-agent-delivery.md`: journal
  event types.

## Results

Pending.
