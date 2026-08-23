---
id: "03-render-running-warning"
title: "Render the notice during running sessions"
status: done
wave: 3
depends_on: ["02-persist-stall-warning"]
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-stall-recovery.md"
---

# Task 03: Render the notice during running sessions

## Acceptance

- `action_visibility: running` messages render while the session is `RUNNING`
  only when their `turn_id` matches the active turn, and hide after settlement
  or when a later turn becomes active.
- Existing recovery messages keep their current visibility behavior.
- The running-only notice is one inline row with muted neutral copy, no alert
  icon, warning/error color, tinted background, border, or stacked action area.
- **Cancel turn** is neutral and content-width at every breakpoint. It stays
  compact on desktop and has a minimum 44px touch height on phones.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/messages/action-message.test.tsx`

The component regression must first fail because all action messages are hidden
during `RUNNING`, then pass with the neutral compact presentation without
weakening ordinary recovery visibility.

## Files likely touched

- `apps/web/components/task/chat/messages/action-message.tsx`
- `apps/web/components/task/chat/messages/action-message.test.tsx`
- `apps/web/components/task/chat/types.ts`

## Dependencies

Task 02.

## Parallelism

Sequential. It consumes the backend metadata contract and precedes E2E.

## Inputs

- Spec desktop/mobile notice scenarios
- Plan frontend section and mobile design contract
- Existing `ActionMessage`, `ActionButtons`, and mobile button sizing; do not
  reuse the stacked full-width mobile action layout for this notice

## Output contract

Report the RED assertion, visibility rule, desktop/mobile presentation,
targeted test result, files changed, blockers, and risks. Mark this task `done`
and update its plan checkbox in the same conversation.
