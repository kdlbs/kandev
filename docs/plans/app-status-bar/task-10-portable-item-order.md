---
id: app-status-bar-10
title: Portable status item order
status: done
wave: 6
depends_on: []
plan: docs/plans/app-status-bar/plan.md
spec: docs/specs/ui/app-status-bar.md
---

# Portable status item order

## Inputs

[Spec: data, API, and persistence](../../specs/ui/app-status-bar.md#data-api-and-persistence),
[ADR 0041](../../decisions/0041-backend-owned-portable-user-settings.md), and
[portable-order ADR](../../decisions/2026-07-21-portable-status-bar-order.md).

## Files likely touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/dto/dto.go`, `dto_test.go`
- `apps/backend/internal/user/service/service.go`, `service_test.go`
- `apps/backend/internal/user/store/sqlite.go`, `sqlite_test.go`
- `apps/backend/internal/user/controller/controller.go`, `controller_test.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/web/lib/types/http-user-settings.ts`, `backend.ts`
- `apps/web/lib/state/slices/settings/types.ts`, `settings-slice.ts`
- `apps/web/lib/ssr/user-settings.ts`, `user-settings.test.ts`
- `apps/web/lib/ws/handlers/users.ts`, `users.test.ts`

## Acceptance

1. Existing user-settings GET/PATCH/event/boot flows round-trip
   `app_status_bar_order.left_item_ids/right_item_ids`; omission leaves the prior
   value unchanged and an absent value maps to empty arrays for host reconciliation.
2. The preference is stored in the existing JSON blob with no relational schema,
   endpoint, browser-storage, or plugin-protocol addition.
3. Backend and frontend tests cover empty/default, explicit replacement, omitted
   PATCH, reload, boot hydration, and live `user.settings.updated` mapping.

## Verification

```sh
cd apps/backend && go test ./internal/user/... ./internal/backendapp/...
cd apps && pnpm --filter @kandev/web test -- lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts
cd apps/web && pnpm run typecheck
```

## Dependencies

None; this can run in parallel with task 11.

## Output contract

Report the exact wire shape, persistence/omission behavior, files changed, red
and green tests, and blockers. Mark the task and plan row done only after focused
verification passes.
