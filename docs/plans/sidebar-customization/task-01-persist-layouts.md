---
id: "01-persist-layouts"
title: "Persist workspace sidebar layouts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-001
  - REQ-UI-SIDEBAR-CUSTOMIZATION-004
acceptance_criteria:
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-004.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-004.5
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
---

# Task 01: Persist workspace sidebar layouts

## Summary

Add a validated scoped user-settings contract and complete all delivery paths. The result is testable through settings reads and writes before UI adoption.

## In scope

- Models, DTOs, service validation and CAS, access filtering, revision conflicts, reset tombstones, defaults, and store round trips.
- HTTP, boot, WebSocket, settings catalog/schema parity, and frontend wire/store mapping.

## Out of scope

- Visual editor, resource execution, and task-view preference changes.

## Acceptance

- Round trips preserve layout order and unrelated settings across restart and scoped concurrent saves.
- Invalid, inaccessible, and stale-revision writes fail atomically; reset does not admit pre-reset writes.
- Boot, HTTP, and WS expose the same defaults and versioned layout data.

## Verification

Run from the repository root. Install dependencies once with
`(cd apps && pnpm install --frozen-lockfile)` if this worktree has no install.
Use TDD for new logic and the named E2E scenarios. Managed E2E commands rebuild
assets; run desktop and phone commands sequentially without worker overrides.

```bash
(cd apps/backend && go test ./internal/user/... ./internal/settingscatalog/...)
(cd apps/web && pnpm exec vitest run lib/ssr/user-settings.test.ts)
(cd apps/web && pnpm run typecheck)
```

## Files likely touched

- `apps/backend/internal/user/{models,dto,service,handlers,store}/`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/settingscatalog/`
- `apps/web/lib/state/slices/settings/`
- `apps/web/lib/ws/handlers/users.ts`
- `apps/web/lib/ssr/user-settings.ts and user-settings.test.ts`

## Dependencies

None.

## Risks

Incomplete settings delivery can make reload or live updates erase preferences. Preserve current identity/access rules and cloned CAS values.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/sidebar-customization.md), IDs above.
- [System design](../../specs/ui/system-design/sidebar-customization.md), corresponding sections.
- [Plan](plan.md), test matrix and previews.
- Scoped `apps/web/AGENTS.md` and `apps/backend/AGENTS.md` where applicable.

## Results

Implemented workspace-scoped sidebar layout models, DTOs, validation, CAS
updates, reset handling, store persistence, boot/HTTP/WS exposure, and frontend
settings mapping. Verification passed with
`(cd apps/backend && go test ./internal/user/... ./internal/settingscatalog/...)`,
the frontend SSR settings tests, and `pnpm run typecheck`.
