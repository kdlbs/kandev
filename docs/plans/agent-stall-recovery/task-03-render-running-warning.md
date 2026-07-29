---
id: "03-render-running-warning"
title: "Render the warning during running sessions"
status: pending
wave: 3
depends_on: ["02-persist-stall-warning"]
plan: "plan.md"
spec: "../../specs/agent-stall-recovery/spec.md"
---

# Task 03: Render the warning during running sessions

## Acceptance

- `action_visibility: running` messages render while the session is `RUNNING`
  and hide after it settles.
- Existing recovery messages keep their current visibility behavior.
- **Cancel turn** uses the shared responsive action presentation: compact on
  desktop and full-width with a minimum 44px touch height on phones.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/messages/action-message.test.tsx`

The component regression must first fail because all action messages are hidden
during `RUNNING`, then pass without weakening ordinary recovery visibility.

## Files likely touched

- `apps/web/components/task/chat/messages/action-message.tsx`
- `apps/web/components/task/chat/messages/action-message.test.tsx`
- `apps/web/components/task/chat/types.ts`

## Dependencies

Task 02.

## Parallelism

Sequential. It consumes the backend metadata contract and precedes E2E.

## Inputs

- Spec desktop/mobile warning scenarios
- Plan frontend section and mobile design contract
- Existing `ActionMessage`, `ActionButtons`, and mobile button sizing

## Output contract

Report the RED assertion, visibility rule, desktop/mobile presentation,
targeted test result, files changed, blockers, and risks. Mark this task `done`
and update its plan checkbox in the same conversation.
