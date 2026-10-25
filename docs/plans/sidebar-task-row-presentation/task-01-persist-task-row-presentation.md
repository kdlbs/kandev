---
id: "01-persist-task-row-presentation"
title: "Persist task-row presentation"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-task-row-presentation.md"
---

# Task 01: Persist Task-Row Presentation

## Acceptance

- Each saved sidebar view and draft can carry a complete task-row presentation through REST,
  SQLite, boot hydration, frontend state, save, save-as, duplicate, discard, and rollback flows.
- Missing or malformed data resolves to the current layout. Explicitly disabled details remain
  disabled after a round trip.
- A draft presentation previews as the effective view without mutating the saved view.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/backend && go test -tags fts5 ./internal/user/... ./internal/backendapp
make -C apps/backend test
cd apps && pnpm --filter @kandev/web test -- --run lib/state/slices/ui/sidebar-view-wire.test.ts lib/state/slices/ui/ui-slice-migration.test.ts lib/state/slices/ui/sidebar-view-actions.test.ts lib/state/slices/ui/ui-slice.test.ts
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/store/sqlite_test.go`
- `apps/backend/internal/user/service/service_test.go`
- `apps/backend/internal/user/handlers/handlers_test.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/backendapp/boot_state_routes_test.go`
- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/state/slices/ui/sidebar-view-types.ts`
- `apps/web/lib/state/slices/ui/sidebar-view-wire.ts`
- `apps/web/lib/state/slices/ui/sidebar-view-builtins.ts`
- `apps/web/lib/state/slices/ui/sidebar-view-actions.ts`
- `apps/web/lib/state/slices/ui/ui-slice.ts`
- `apps/web/hooks/domains/sidebar/use-effective-sidebar-view.ts`
- Focused tests beside these files

## Dependencies

None.

## Parallelism

`parallel-safe` with Task 04. This task owns the saved-view contract and state files. Task 04 owns
the provider-neutral summary component.

## Inputs

- The data and persistence rules in the sidebar task-row presentation spec.
- ADR 0041 backend ownership and complete-payload normalization rules.
- Existing sidebar view wire, migration, optimistic mutation, and boot-state mapping paths.
- The canonical current layout: details enabled, all fields visible in current order, and Git
  changes on the right.

## TDD Sequence

1. Add frontend normalization and wire tests for missing, valid, malformed, duplicate, and future
   fields. Record the expected failures.
2. Add action tests for draft creation, save, save-as, duplicate, discard, and failed-write rollback.
3. Add backend persistence and boot-map tests for an explicit presentation and a legacy missing
   object. Record the expected failures.
4. Add the nullable backend model and explicit frontend API and domain types.
5. Implement the pure frontend normalizer and extend wire, migration, built-in, action, and
   effective-view paths.
6. Run focused backend and frontend tests, then frontend typecheck.
7. Record exact results and synchronize the plan and task status.

## Risks

- A plain boolean zero value cannot distinguish legacy missing data from explicit `false`.
- Full-array optimistic rollback must restore views, active view, and draft together.
- A future client can add a field that this client does not know. Unknown values must not corrupt
  known order or visibility.

## Output Contract

Report RED failures, the final wire shape, normalization decisions, files changed, exact test
results, compatibility behavior, blockers, risks, and synchronized task and plan status.

## Results

RED coverage was added for missing and explicit task-row wire data before the production model and
normalizer were implemented. The backend model, SQLite defaults/round trip, boot mapping, API wire
mapping, migration, effective-view preview, and optimistic view actions now preserve the complete
normalized object. Targeted backend coverage passed with 808 tests across 7 packages; frontend
persistence coverage passed with 61 tests across 4 files; frontend typecheck passed.
