---
id: "02-frontend-behavior"
title: "Frontend archive confirmation behavior"
status: done
wave: 2
depends_on: ["01-backend-preference"]
plan: "plan.md"
spec: "../../specs/tasks/archive-confirmation.md"
---

# Task 02: Frontend Archive Confirmation Behavior

## Acceptance

- General settings exposes a default-enabled archive confirmation switch with optimistic save and rollback.
- All UI archive surfaces continue to confirm while enabled and immediately archive with `cascade: false` while disabled.
- Missing boot/API/WS values resolve to confirmation enabled.

## Verification

`cd apps && pnpm --filter @kandev/web test -- --run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts components/task/task-archive-confirm-dialog.test.tsx components/settings/general-settings.test.tsx`

## Files Likely Touched

- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/state/slices/settings/settings-slice.ts`
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/lib/ws/handlers/users.ts`
- `apps/web/components/settings/general-settings.tsx`
- `apps/web/components/task/task-archive-confirm-dialog.tsx`
- Nearby focused tests.

## Inputs

Backend task 01 contract; spec What, Failure modes, and Scenarios. Reuse the optimistic switch pattern from changelog and notification settings.

## Output Contract

Report state/UI/dialog behavior, tests run, files touched, blockers, and risks; mark this task and the plan entry done.
