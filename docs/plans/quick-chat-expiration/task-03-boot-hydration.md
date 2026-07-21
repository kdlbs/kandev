---
id: "03-boot-hydration"
title: "Boot hydration"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/quick-chat-expiration.md"
---

# Task 03: Boot Hydration

## Acceptance

- SPA boot payload includes `initialState.quickChat.sessions` for the active workspace whenever an active workspace can be resolved.
- Restored sessions exclude config-mode and automation-run ephemeral tasks, require a primary session, and are ordered by newest last activity first.
- Frontend hydration keeps existing local quick-chat rename overlays and remains type-correct.

## Verification

- `cd apps/backend && go test ./internal/backendapp -run 'TestBoot.*QuickChat|TestBootPayload'`
- `cd apps && pnpm --filter @kandev/web test -- --run lib/state/hydration/hydrator.test.ts src/boot-payload.test.ts`
- `cd apps/web && pnpm run typecheck`

## Files likely touched

- `apps/backend/internal/backendapp/boot_state.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/backendapp/helpers_test.go`
- `apps/web/lib/api/domains/workspace-api.ts`
- `apps/web/app/page.tsx`
- `apps/web/lib/state/hydration/hydrator.test.ts`
- Optional `apps/web/src/boot-payload.test.ts`

## Dependencies

None. This can proceed in parallel with Task 01 because it can use existing `ListTasksByWorkspace` plus session enrichment.

## Inputs

- Spec sections: API surface, Persistence guarantees, Scenarios.
- Existing patterns: home/tasks boot-state helpers in `apps/backend/internal/backendapp/boot_state.go` and `boot_state_routes.go`; quick-chat local rename overlay in `apps/web/lib/state/hydration/hydrator.ts`.

## Output contract

Update this task status to `done`, update the Wave 1 checkbox in `plan.md`, and report the files changed, tests run, and any routes where quick-chat hydration intentionally no-ops.
