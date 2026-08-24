---
id: app-status-bar-appearance-01
title: Portable status bar visibility contract
status: done
wave: 1
depends_on: []
plan: docs/plans/app-status-bar-appearance-setting/plan.md
spec: docs/specs/ui/requirements/app-status-bar.md
decision: docs/decisions/2026-08-11-user-owned-status-bar-visibility.md
---

# Portable status bar visibility contract

## Inputs

The approved [App Status Bar spec](../../specs/ui/requirements/app-status-bar.md), ADR 0041
backend-owned portable settings, the new visibility ADR, and the existing
`app_status_bar_order` user-settings round trip.

## TDD sequence

1. Add failing backend tests for missing-value default false, explicit-true
   round trip, PATCH omission versus explicit values, event data, boot mapping,
   atomic revisions, and legacy-schema migration.
2. Add failing frontend mapper/handler tests for default false, explicit true,
   and omission preserving current state.
3. Add the smallest model, DTO, service, storage, boot, HTTP type, state, and
   mapper changes that make those tests pass.
4. Refactor only repeated mapping code exposed by the tests. Do not change live
   status surface gates in this task.

## Implementation

- Add `AppStatusBarEnabled bool` to
  `apps/backend/internal/user/models/models.go`.
- Add the response field and pointer PATCH field to
  `apps/backend/internal/user/dto/dto.go`, including `FromUserSettings`.
- Thread the PATCH pointer through
  `apps/backend/internal/user/controller/controller.go` and
  `apps/backend/internal/user/service/service.go`.
- Apply an explicit value only when the pointer is non-nil. Publish
  `app_status_bar_enabled` in the complete `user.settings.updated` event.
- In `apps/backend/internal/user/store/sqlite.go`:
  - set `AppStatusBarEnabled: false` in `defaultUserSettings`;
  - include the field in JSON writes;
  - decode it as `*bool`;
  - overwrite the default only when the stored pointer is present;
  - migrate a `settings_revision` column and atomically increment and return it
    with every settings mutation.
- Add `appStatusBarEnabled` to
  `apps/backend/internal/backendapp/boot_state_routes.go`.
- Carry the numeric revision through the response DTO, boot state, and complete
  settings event so clients can order snapshots without timestamps.
- Add `app_status_bar_enabled?: boolean` to response and PATCH shapes in
  `apps/web/lib/types/http-user-settings.ts`.
- Add `appStatusBarEnabled: boolean` to
  `apps/web/lib/state/slices/settings/types.ts`.
- Initialize it to false and map it through the shared paths in
  `apps/web/lib/ssr/user-settings.ts`. The common mapper must preserve the
  current value for partial updates.
- Update the existing user WebSocket handler and ensure-user-settings fixtures
  through their shared mapper; do not add a second field-specific handler.
- Apply one revision comparison at boot, HTTP, and WebSocket ingestion points;
  reject older snapshots without assigning them a newer local revision.

The only SQL migration adds the per-user settings revision counter. No browser
storage is allowed.

## Files likely touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/dto/dto.go`
- `apps/backend/internal/user/dto/dto_test.go`
- `apps/backend/internal/user/controller/controller.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/service/service_test.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/store/sqlite_test.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/backendapp/boot_state_user_settings_test.go`
- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/lib/ssr/user-settings.test.ts`
- `apps/web/lib/ws/handlers/users.test.ts`
- `apps/web/hooks/use-ensure-user-settings.test.ts`

## Acceptance

1. A new user, an old settings JSON object with no visibility field, and an
   initial compatibility payload that omits it all resolve to false.
2. Stored and PATCHed true remains true through repository read, DTO response,
   boot state, frontend mapping, reload, and event mapping.
3. An omitted PATCH field leaves the existing setting unchanged.
4. A partial WebSocket payload without the field preserves the current
   frontend value.
5. The backend emits `app_status_bar_enabled` in the complete user-settings
   event and `appStatusBarEnabled` in boot state.
6. Concurrent settings writes receive distinct increasing revisions, and an
   existing database without the revision column upgrades cleanly.
7. Runtime feature behavior remains intact until Tasks 02 and 03.

## Verification

```sh
(cd apps/backend && go test ./internal/user/... ./internal/backendapp/...)
(cd apps && pnpm --filter @kandev/web exec vitest run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts hooks/use-ensure-user-settings.test.ts)
git diff --check
```

## Dependencies

None. This task creates the portable contract consumed by Task 02.

## Risks

- A default that remains true in either mapper would enable the surface for old
  rows that have no preference.
- Defaulting independently inside several frontend consumers can make boot and
  live updates disagree. Use the shared mapper.
- A complete event map can silently omit the new field even when HTTP works.
  Pin the event key in service tests.

## Output contract

Report the exact default/false/omission tests, API and event shapes, files
changed, commands run, and blockers. Mark this task done only when the portable
setting round trip is green and live UI remains on the old gate.
