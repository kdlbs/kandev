---
id: "01-persist-automatic-color-settings"
title: "Persist automatic color settings"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004
acceptance_criteria:
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.11
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.13
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.1
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.2
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.3
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.4
system_design:
  - ../../specs/ui/system-design/sidebar-automatic-task-colors.md
---

# Task 01: Persist Automatic Color Settings

## Summary

Add a typed, bounded automatic-color object to portable user settings. Store the object in the backend database and add one revision-aware frontend mutation path.

## In scope

- Go models, DTOs, controller mapping, service patch validation, defaults, and stored JSON.
- Frontend API types, settings state, normalization, boot mapping, and WebSocket mapping.
- A serialized optimistic mutation helper with safe rollback and localized error state.
- Exact limits for rule count, IDs, labels, identity fields, and local paths.
- Disabled incomplete rules and discriminated fixed or workflow-step outputs.

## Out of scope

- Rule matching.
- Repository discovery.
- Editor controls.

## Acceptance

- Missing settings produce disabled automation with an empty rule list.
- The `users.settings` JSON value preserves rule order, targets, stored labels, and output selections.
- The backend preserves disabled incomplete rules. It rejects enabled incomplete rules, malformed values, oversized fields, and more than 50 rules.
- Successful and failed writes preserve revision order and unrelated settings.

## Verification

```bash
(cd apps/backend && go test ./internal/user/...)
(cd apps/web && pnpm exec vitest run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts lib/state/default-state.test.ts lib/task-color-automation-settings.test.ts)
```

## Files likely touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/dto/dto.go`
- `apps/backend/internal/user/controller/controller.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/**/*_test.go`
- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/lib/settings/user-settings-revision.ts`
- `apps/web/lib/ws/handlers/users.ts`
- `apps/web/lib/task-color-automation-settings.ts`

## Dependencies

None.

## Risks

- A shallow settings copy can alias rule slices. Apply helpers must replace slices.
- A failed older write must not roll back a newer confirmed write.

## Parallelism

`sequential`

## Inputs

- Requirement sections for ordered rules and portable settings.
- System-design sections for the rule model and settings persistence.
- Existing sidebar-view write journal and common user-settings mapper.

## Results

Implemented the bounded Go user-settings model, DTO/controller/service/store/event plumbing, defaults, corrupt-value isolation, and revision-safe complete replacement. Added the web wire/state types, SSR and WebSocket hydration, serialized optimistic mutation hook, and regression coverage for persistence and hydration.

Verification: `go test ./internal/user/...` passed; the focused frontend user-settings and parser suite passed 52 tests; the full focused backend package run passed 3,282 tests.
