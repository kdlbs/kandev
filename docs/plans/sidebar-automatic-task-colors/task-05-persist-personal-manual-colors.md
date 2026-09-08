---
id: "05-persist-personal-manual-colors"
title: "Persist personal manual colors"
status: done
wave: 5
depends_on:
  - "04-deliver-responsive-automatic-color-editor"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005
acceptance_criteria:
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.7
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.8
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.12
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005.1
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005.2
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005.4
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005.5
system_design:
  - ../../specs/ui/system-design/sidebar-automatic-task-colors.md
---

# Task 05: Persist Personal Manual Colors

## Summary

Add a backend-owned manual-color map to personal user settings. Add an atomic patch that supports normal edits, clear tombstones, and import-only-missing writes.

## In scope

- Go models, DTOs, controller mapping, service patch validation, stored JSON, boot state, and settings events.
- Seven valid manual color values, clear tombstones, and safe defaults for old settings.
- A bounded `sidebar_task_color_patch` request with normal and `if_missing` behavior.
- CAS retries that reapply per-task changes to the latest settings map.
- Frontend wire types, normalized settings state, and common HTTP or WebSocket mapping.

## Out of scope

- Reading browser storage.
- Replacing the current manual-color hook.
- New colors or changes to task records.

## Acceptance

- The backend stores personal colors and tombstones without changing task records.
- Normal patches affect only supplied tasks. Import patches never overwrite a color or tombstone.
- Concurrent patches for different tasks survive CAS retries without lost updates.
- One patch accepts 500 entries. The stored map accepts 10,000 entries and 1 MiB of encoded JSON.

## Verification

```bash
(cd apps/backend && go test ./internal/user/... ./internal/backendapp)
(cd apps/web && pnpm exec vitest run lib/ssr/user-settings.test.ts lib/ssr/user-settings-task-colors.test.ts lib/ws/handlers/users.test.ts lib/task-colors.test.ts)
```

## Files likely touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/models/sidebar_task_colors.go`
- `apps/backend/internal/user/dto/dto.go`
- `apps/backend/internal/user/controller/controller.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/backendapp/boot_state.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/user/**/*_test.go`
- `apps/backend/internal/backendapp/*user_settings_test.go`
- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/lib/ws/handlers/users.ts`
- `apps/web/lib/task-colors.ts`

## Dependencies

Task 04 supplies the current color palettes, effective-color precedence, and user-settings event path.

## Risks

- A full-map replacement can erase an unrelated browser edit.
- A shallow settings copy can alias the stored map during change detection.
- Unbounded tombstones can enlarge every settings response.

## Parallelism

`sequential`

## Inputs

- Requirement sections for manual color ownership, migration precedence, and portability.
- System-design sections for the manual color model and mutation.
- ADR 0041 for backend-owned portable settings.

## Results

Implemented the personal manual-color map and bounded per-task patch across the Go model, DTO, controller, service, SQLite settings payload, boot state, and settings events. Normal edits overwrite supplied task IDs, clear edits retain tombstones, and missing-only imports preserve existing decisions. Added the web wire types, normalized hydration, and backend regression coverage.

Verification: `go test ./internal/user/... ./internal/backendapp` passed 1,158 tests across 7 packages; the focused settings and color suite passed 88 tests.
