---
id: "01-publish-compact-session-attention"
title: "Publish compact session attention"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-VIEWPORT-SESSION-DELIVERY-002
acceptance_criteria:
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-002.1
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-002.2
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-002.3
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-002.4
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-002.5
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-002.6
system_design:
  - ../../specs/platform/system-design/viewport-bounded-session-delivery.md
---

# Task 01: Publish compact session attention

## Summary

Publish per-session pending clarification and permission changes through a
compact workspace event. Keep message content session-scoped and merge compact
status with the existing revision-aware task-session projection.

## In scope

- Add the internal semantic event and public WebSocket action
  `session.pending_action_changed`.
- Publish a complete pending-action replacement after authoritative pending
  projection changes.
- Route the event to the owning workspace and fail closed when workspace
  identity is unavailable.
- Add frontend action typing, registration, and a handler that reuses
  `setTaskSessionPendingAction` revision ordering.
- Preserve existing workspace-scoped lifecycle/activity updates and
  session-scoped message delivery.
- Add backend payload/routing and frontend stale-revision tests before
  production changes.

## Out of scope

- Transcript delivery, task-summary schema, persistence migrations, polling,
  session list redesign, or Threads layout.

## Acceptance

- Clarification, permission, and clear transitions publish exact session,
  task, workspace, value, and revision fields without message content.
- Only authorized clients in the owning workspace receive the compact event.
- The frontend updates loaded or keyed compact session projection and ignores
  older revisions.

## Verification

Start with focused tests that fail because the action and routing do not exist.
Then run:

```bash
(cd apps/backend && rtk go test ./internal/task/service ./internal/gateway/websocket ./pkg/websocket -run 'PendingAction|TaskEventBroadcaster|SessionPending' -race)
(cd apps && pnpm --filter @kandev/web test -- --run lib/ws/handlers/session-pending-action.test.ts lib/state/slices/session/update-session-read-cursor.test.ts)
```

## Files likely touched

- `apps/backend/internal/events/types.go`
- `apps/backend/internal/task/service/service_events.go`
- `apps/backend/internal/gateway/websocket/task_notifications.go`
- `apps/backend/internal/gateway/websocket/task_notifications_test.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/ws/handlers/types.ts`
- `apps/web/lib/ws/handlers/index.ts`
- `apps/web/lib/ws/handlers/session-pending-action.ts`
- `apps/web/lib/ws/handlers/session-pending-action.test.ts`
- `apps/web/lib/state/slices/session/task-session-projection-actions.ts`

## Dependencies

None.

## Risks

- Do not route the existing message event to a workspace. Build a separate
  compact payload.
- Preserve the current epoch and sequence comparison across list, message, and
  compact-event sources.

## Parallelism

`sequential`

## Results

Implemented and verified. The compact workspace event is emitted for pending
action replacements, routed only to the owning workspace, and consumed by the
revision-aware frontend session projection. Focused backend and frontend
checks pass.

The review fix-up retains orphan projections until the session list hydrates,
rejects stale successors, and suppresses unchanged pending-action fan-out
after message events. The backend race suite passes 33 matching tests.
