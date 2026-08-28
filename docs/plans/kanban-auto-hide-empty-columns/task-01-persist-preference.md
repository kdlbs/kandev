---
id: "01-persist-preference"
title: "Persist the per-workflow auto-hide preference"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/kanban-auto-hide-empty-columns.md"
---

# Task 01: Persist the per-workflow auto-hide preference

## Acceptance

- User settings persist a sorted, deduplicated `workflow_ids_with_auto_hide_empty_steps` list and
  hydrate it as `workflowIdsWithAutoHideEmptySteps`, defaulting missing state to `[]`.
- The existing Columns menu exposes an accessible, translated per-workflow toggle on desktop and
  phone without modifying `hiddenWorkflowStepIds`.
- Two workflows can hold different values, and toggling one never changes the other.

## Likely files

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/dto/dto.go` and focused tests
- `apps/backend/internal/user/controller/controller.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/state/default-state.ts`
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/hooks/use-user-display-settings.ts` and tests
- `apps/web/hooks/use-kanban-display-settings.ts` and tests
- `apps/web/components/kanban/columns-menu.tsx` and tests
- `apps/web/src/locales/*/kanban.json`

## Verification

```bash
make -C apps/backend test
(cd apps && pnpm --filter @kandev/web test -- --run hooks/use-user-display-settings.test.ts hooks/use-kanban-display-settings.test.ts components/kanban/columns-menu.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
```

## Risks

- The boot-state key must match the Zustand field exactly.
- Empty values must serialize authoritatively so clearing the preference cannot preserve stale state.
- The control needs complete translations in every gated locale.
