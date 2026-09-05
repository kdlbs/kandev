---
id: "04-stabilize-session-transport"
title: "Stabilize session transport"
status: completed
wave: 4
depends_on: ["03-decouple-git-polling"]
plan: "plan.md"
spec: "../../specs/platform/requirements/bounded-task-status-delivery.md"
---

# Task 04: Stabilize Session Transport

## Acceptance

- Correlated responses/errors use a reserved priority queue; notification
  saturation cannot silently drop them, and control overflow closes the socket
  with an observable reason.
- `session.subscribe` sends a full snapshot only on new membership and
  `session.focus` never replays one; repeated requests remain acknowledged and
  idempotent.
- `session.git.refresh` refreshes only selected-session Git data, replacing the
  frontend's focus-as-refresh workaround without changing focus membership.

## TDD sequence

1. RED: saturate notification delivery and prove a correlated response is the
   next deliverable control frame; prove control overflow closes explicitly.
2. RED: add duplicate subscribe/focus tests that count full snapshot actions,
   plus targeted Git refresh tests that exclude model/message/shell snapshots.
3. GREEN: split client outbound queues, prioritize bounded batches in the write
   pump, expose membership transitions, and add the targeted action.
4. GREEN: migrate `refreshSessionData` callers to the targeted Git refresh.
5. REFACTOR: add queue/drop/close telemetry and preserve FIFO order within each
   traffic class without permanently starving notifications.

## Verification

```bash
cd apps/backend && go test ./internal/gateway/websocket/...
cd apps && pnpm --filter @kandev/web test -- --run lib/ws
```

## Files likely touched

- `apps/backend/internal/gateway/websocket/client.go`
- `apps/backend/internal/gateway/websocket/hub.go`
- `apps/backend/internal/gateway/websocket/hub_session_mode.go`
- `apps/backend/internal/gateway/websocket/client_session_focus_test.go`
- new gateway queue/saturation tests
- shared WebSocket action definitions
- `apps/web/lib/ws/client.ts`
- `apps/web/lib/types/backend.ts`
- active-session Git refresh callers such as Dockview/shared session hooks
- focused frontend WS tests

## Dependencies

Task 03 ensures removing viewer interest does not pause live Git monitoring.

## Parallelism

Sequential. Task 05 consumes the final subscription contract; Task 06 relies
on response-delivery semantics.

## Inputs

- Spec **API and event surface** WebSocket clauses.
- Existing `Client.send`, `WritePump`, `handleSessionSubscribe`,
  `handleSessionFocus`, and `sendSessionData` behavior.
- Existing `session-focus-signal.spec.ts` contract, which must be updated.

## Risks

- Absolute control priority can starve notifications; use a bounded priority
  burst and test fairness.
- A duplicate frontend ref-count subscribe must still receive an ACK even when
  no snapshot is replayed.
- Closing on control overflow must release hub membership and trigger normal
  client reconciliation.

## Verification results

- `cd apps/backend && go test ./internal/gateway/websocket/...` — passed.
- Gateway tests cover duplicate subscribe/focus ACK-only behavior, targeted
  Git refresh, reserved control delivery under notification pressure, and
  explicit control-queue overflow closure.
- The frontend now uses `session.git.refresh` for selected detail Git refresh;
  focus no longer replays a full session snapshot.

## Output contract

Report queue semantics/capacities, saturation evidence, subscription snapshot
counts, targeted refresh behavior, focused test results, and telemetry added.
