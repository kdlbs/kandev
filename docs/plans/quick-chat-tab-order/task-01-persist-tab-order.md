---
id: "01-persist-tab-order"
title: "Persist tab order"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-QUICK-TERMINAL-002
acceptance_criteria:
  - AC-UI-QUICK-TERMINAL-002.1
  - AC-UI-QUICK-TERMINAL-002.4
  - AC-UI-QUICK-TERMINAL-002.5
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
---

# Task 01: Persist Tab Order

## Summary

Store one portable mixed-tab order for each workspace. Resolve it against current conversation and
terminal membership without changing lifecycle ownership.

## In scope

- Add the user-settings field and all boot, PATCH, event, and hydration projections.
- Replace activity-based conversation order with a stable creation baseline.
- Add the order resolver, optimistic state, serialized saves, and error state.

## Out of scope

- Drag sensors and tab action menus.
- Rename presentation.
- Agent-generated titles.

## Acceptance

- Backend and frontend settings round-trip a workspace order without losing sibling settings.
- The resolver ignores invalid references and appends new persisted tabs in stable order.
- A failed save preserves membership and the active tab while it shows a sync error.

## Verification

```bash
(cd apps/backend && go test ./internal/user/... ./internal/task/service ./internal/backendapp -count=1)
(cd apps/web && pnpm vitest run lib/ssr/user-settings.test.ts lib/state/hydration/hydrator.test.ts lib/state/slices/ui)
(cd apps/web && pnpm run typecheck)
```

## Files likely touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/dto/dto.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/task/service/quick_chat_sessions.go`
- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/lib/state/default-state.ts`
- `apps/web/lib/state/slices/ui/`

## Dependencies

None.

## Risks

- Settings events and responses can arrive out of order. The existing settings revision remains
  authoritative, and local writes must be serialized.

## Parallelism

`parallel-safe` with Task 05.

## Inputs

- UI system design sections for tab identity, order resolution, persistence, and errors.
- The sidebar task-preference save queue and the existing Quick Chat reconciliation tests.

## Results

Implemented the stable creation-order baseline, per-workspace user-settings field, boot and
hydration mapping, unknown-reference resolver, optimistic order state, and serialized save queue.
The backend and frontend focused checks pass.
