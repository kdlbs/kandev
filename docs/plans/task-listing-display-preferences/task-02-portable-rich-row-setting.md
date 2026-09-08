---
id: "02-portable-rich-row-setting"
title: "Portable richer-row setting contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/task-listing-display-preferences.md"
role: implementer
model_tier: balanced
---

# Task 02: Portable richer-row setting contract

## Acceptance

- User-settings GET, PATCH, event, persistence, and boot payload expose
  `tasks_list_show_details` / `tasksListShowDetails`.
- The default is false; explicit true and false values round-trip; omission
  leaves the current value unchanged.
- Backend and frontend mapping tests fail before and pass after the contract
  change.

## Verification

- `make -C apps/backend fmt`
- `cd apps/backend && go test ./internal/user/... ./internal/backendapp/...`
- `cd apps && pnpm --filter @kandev/web test -- --run lib/ssr/user-settings.test.ts`

## Files likely touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/dto/dto.go`
- `apps/backend/internal/user/dto/dto_test.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/service/service_test.go`
- `apps/backend/internal/user/store/sqlite.go`
- closest user-store round-trip test
- `apps/backend/internal/backendapp/boot_state_routes.go`
- closest boot-state route test
- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/state/slices/settings/settings-slice.ts`
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/lib/ssr/user-settings.test.ts`
- closest settings-slice test

## Inputs

- Spec: **Portable richer-row preference**, **API surface**, **Failure modes**,
  and **Persistence guarantees**.
- Plan: **Portable richer-row user setting** and **Portable richer-row frontend
  setting** mappings only.
- ADR: `docs/decisions/0041-backend-owned-portable-user-settings.md`.

## Output contract

Return a compact handoff with intent/acceptance, base/head SHA if committed,
changed files and entry points, spec sections, `public-contract + persistence`
risk tags, exact command results, uncertainties, and the task-file status
update. Do not edit `plan.md`.
