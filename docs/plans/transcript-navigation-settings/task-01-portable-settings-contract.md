---
id: "01-portable-settings-contract"
title: "Portable settings contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/transcript-navigation-settings.md"
---

# Task 01: Portable Settings Contract

## Acceptance

- Backend user-settings GET/PATCH, stored JSON, settings events, and boot state expose
  `show_transcript_auto_scroll_control`.
- Missing anchored-prompt and scroll-to-start fields resolve to `false`; missing
  scroll-to-last-prompt and auto-scroll-control fields resolve to `true`, while explicit values are
  preserved.
- Frontend HTTP/boot/websocket mappings use the same defaults and hydrate the new field.

## Verification

```bash
make -C apps/backend test
cd apps && pnpm --filter @kandev/web test -- --run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts hooks/use-ensure-user-settings.test.ts
```

## Files Likely Touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/dto/dto.go`
- `apps/backend/internal/user/controller/controller.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/service/service_test.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/store/sqlite_test.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/state/slices/settings/settings-slice.ts`
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/lib/ssr/user-settings.test.ts`
- `apps/web/lib/ws/handlers/users.ts`
- `apps/web/lib/ws/handlers/users.test.ts`
- `apps/web/hooks/use-user-display-settings.ts`
- `apps/web/hooks/use-ensure-user-settings.test.ts`
- `apps/web/components/settings/editors-settings-state.tsx`

## Dependencies

None.

## Parallelism

Sequential. Backend and frontend mappings define one shared contract and must land together.

## Inputs

- Spec: Data Model, API Surface, Persistence Guarantees, and default-value scenarios.
- ADR 0041: backend-owned portable user settings.
- Existing transcript-navigation boolean fields and pointer-backed SQLite scan pattern.

## Risks

- A plain boolean in stored JSON cannot distinguish absent from explicit false; scan payload fields
  must remain pointers.
- Boot, SSR, websocket, and nested settings mappings must agree on fallbacks.

## Output Contract

Report the contract/default changes, files changed, exact test results, blockers and residual risks,
then mark this task `done` and update `plan.md`.

## Result

- Added the backend-owned `show_transcript_auto_scroll_control` preference to stored JSON, GET/PATCH DTOs,
  service/controller mappings, settings events, and boot state.
- Changed absent-value defaults for anchored prompt and scroll-to-start to `false`; scroll-to-last-prompt
  and the new auto-scroll-control visibility remain `true`.
- Carried the contract through frontend state, HTTP/WS types, SSR mapping, live websocket updates,
  display-settings carry-forward, and nested settings response mapping.
- `cd apps/backend && go test ./internal/user/... ./internal/backendapp/...` — passed (381 tests, 7 packages).
- `cd apps && pnpm --filter @kandev/web test -- --run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts hooks/use-ensure-user-settings.test.ts` — passed (47 tests, 3 files).
