---
id: "01-persist-thread-views"
title: "Persist Threads saved views"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-SAVED-VIEWS-001
  - REQ-UI-THREADS-SAVED-VIEWS-002
  - REQ-UI-THREADS-SAVED-VIEWS-003
  - REQ-UI-THREADS-SAVED-VIEWS-004
acceptance_criteria:
  - AC-UI-THREADS-SAVED-VIEWS-001.2
  - AC-UI-THREADS-SAVED-VIEWS-001.4
  - AC-UI-THREADS-SAVED-VIEWS-001.5
  - AC-UI-THREADS-SAVED-VIEWS-001.6
  - AC-UI-THREADS-SAVED-VIEWS-001.7
  - AC-UI-THREADS-SAVED-VIEWS-001.8
  - AC-UI-THREADS-SAVED-VIEWS-001.9
  - AC-UI-THREADS-SAVED-VIEWS-004.9
  - AC-UI-THREADS-SAVED-VIEWS-004.10
system_design:
  - ../../specs/ui/system-design/threads-saved-views.md
---

# Task 01: Persist Threads Saved Views

## Summary

Add a separate Threads saved-view collection, active ID, and draft to portable
user settings. Synchronize them through the existing boot, PATCH, revision,
and live-event paths.

## In scope

- Add backend models and DTOs for Threads view definitions.
- Validate view count, IDs, names, active references, clauses, selected task
  IDs, and column limits.
- Preserve omitted fields during a partial settings update.
- Add frontend saved-view, task-scope, filter, sort, and draft types.
- Normalize snake-case wire data and canonical defaults.
- Add UI-slice actions with queued writes, optimistic rollback, and revision
  rejection.
- Hydrate state from boot data, HTTP responses, and user-settings events.
- Extract shared query primitives without changing sidebar wire keys or view
  behavior.

## Out of scope

- Candidate filtering, Threads header UI, and Playwright coverage.

## Acceptance

- A user can persist up to 50 valid Threads views.
- The canonical view appears when stored fields are absent or empty.
- Threads and sidebar active IDs and drafts remain independent.
- Another client receives accepted settings through the normal live event.
- A failed update restores the last backend snapshot and exposes a sync error.

## Verification

Write failing model, service, repository, mapper, and store tests first. Then
run:

```bash
(cd apps/backend && go test ./internal/user/...)
(cd apps && pnpm --filter @kandev/web test -- --run lib/state/slices/ui/thread-view-actions.test.ts lib/state/slices/ui/thread-view-wire.test.ts lib/state/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts)
(cd apps/web && pnpm run typecheck)
```

## Files likely touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/dto/user_settings.go`
- `apps/backend/internal/user/service/user_service.go`
- `apps/backend/internal/user/store/user_store.go`
- `apps/backend/internal/user/controllers/user_controller.go`
- `apps/web/lib/http/types.ts`
- `apps/web/lib/state/slices/ui/thread-view-types.ts`
- `apps/web/lib/state/slices/ui/thread-view-actions.ts`
- `apps/web/lib/state/slices/ui/thread-view-wire.ts`
- `apps/web/lib/state/ssr/user-settings.ts`
- `apps/web/lib/ws/handlers/users.ts`

## Dependencies

None.

## Risks

- Do not replace unrelated user-settings fields during a Threads-only PATCH.
- Do not share IDs or the active selection with sidebar views.

## Parallelism

`sequential`
