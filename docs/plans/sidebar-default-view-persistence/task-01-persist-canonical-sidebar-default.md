---
id: "01-persist-canonical-sidebar-default"
title: "Persist the canonical sidebar default"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-view-creation.md"
---

# Task 01: Persist the canonical sidebar default

Implement the backend-owned omitted-value default and prove the first sidebar
filter edit through unit, HTTP integration, and browser coverage.

## Acceptance

- Effective settings whose stored JSON omits sidebar-view fields contain one
  canonical `All tasks` view with ID `view-all-tasks` and the same active ID;
  unrelated stored fields and explicit sidebar values are preserved.
- A `sidebar_views: []` write that omits the active ID normalizes back to the
  canonical view instead of persisting an invalid active/view pair.
- A PATCH containing only `sidebar_active_view_id="view-all-tasks"` and a
  sidebar draft succeeds from a clean user and persists the draft.
- Editing the first default view in the browser produces no sidebar settings
  toast and survives reload; stale active IDs remain rejected.

## TDD sequence

1. Add the clean-user HTTP regression test and run it before production edits;
   record the expected HTTP 400 caused by the missing saved view.
2. Add the store default/overlay and empty-replacement service/HTTP cases, then
   implement the minimal fresh canonical default plus presence-aware scan
   overlay and write normalization.
3. Rerun the targeted Go tests until both HTTP regressions return 200 and the
   existing stale-ID validation test remains green.
4. Align the E2E reset fixture with the canonical backend state, extend the
   first-edit browser scenario, and run only that scenario.

## Files likely touched

- `docs/specs/ui/requirements/sidebar-view-creation.md`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/store/sqlite_test.go`
- `apps/backend/internal/user/handlers/handlers_test.go`
- `apps/web/e2e/fixtures/test-base.ts`
- `apps/web/e2e/tests/task/sidebar-filter.spec.ts`
- `docs/plans/sidebar-default-view-persistence/plan.md`
- `docs/plans/sidebar-default-view-persistence/task-01-persist-canonical-sidebar-default.md`

## Dependencies

None.

## Parallelism

`sequential`. Backend defaults, the shared E2E fixture, and the browser
regression describe one cross-layer contract and should be landed together.

## Inputs

- Spec: `docs/specs/ui/requirements/sidebar-view-creation.md`, especially **Persistence and
  failure behavior** and the clean-user scenarios.
- Decision: `docs/decisions/0041-backend-owned-portable-user-settings.md`.
- Backend paths: `defaultUserSettings`, `scanUserSettings`,
  `applySidebarViewState`, and `Handlers.httpUpdateUserSettings`.
- Frontend consumer: `updateSidebarDraft` in
  `apps/web/lib/state/slices/ui/sidebar-view-actions.ts`.

## Verification

From the repository root:

```bash
cd apps/backend && go test -tags fts5 -run 'TestScanUserSettingsSidebarDefaults|TestScanUserSettingsPreservesExplicitEmptySidebarSettings|TestHTTPUpdateSidebarDraftFromCleanSettings|TestApplySidebarViews|TestApplySidebarViewState' ./internal/user/store ./internal/user/handlers ./internal/user/service -v
```

Result: 23 tests passed across 3 packages.

The worktree was missing the Playwright/Vitest links, so dependencies were
bootstrapped once:

```bash
cd apps && pnpm install --frozen-lockfile
```

Additional scoped checks:

```bash
cd apps/backend && go test -tags fts5 ./internal/user/...
```

Result: 227 tests passed across 6 packages.

```bash
cd apps && pnpm --filter @kandev/web run typecheck
cd apps && pnpm --filter @kandev/web test -- lib/state/slices/ui/ui-slice.test.ts -t "syncs filter sort and group drafts"
cd apps && pnpm --filter @kandev/web lint -- e2e/fixtures/test-base.ts e2e/helpers/api-client.ts e2e/tests/task/sidebar-filter.spec.ts
```

Results: typecheck and lint passed; the focused unit test passed (1 passed, 41
skipped).

The focused browser test was run through the managed runner, which builds and
tears down the real backend/browser fixture:

```bash
cd apps/web && pnpm e2e:run --host --project chromium tests/task/sidebar-filter.spec.ts -- --grep "default 'All tasks' view"
```

Result: 1 Playwright test passed in 5.3s against the rebuilt backend. The first
run exposed a test-overlay timing defect after leaving the view picker open;
the test now closes the picker before editing and the rerun passes. The runner
exited cleanly with no leftover E2E backend/browser processes.

```bash
git diff --check
```

Result: passed.

## Output contract

Report the root-cause regression test's red result, the files changed, every
exact verification command and outcome/count, any generated artifacts or E2E
teardown evidence, remaining risks, and blockers. Update this task to `done`
and synchronize `plan.md` only after all acceptance criteria pass.

## Results

- RED store regression before the production change: `rtk go test -tags fts5 -run TestScanUserSettingsSidebarDefaults ./internal/user/store -v` failed because `{}` and unrelated settings returned zero sidebar views instead of one canonical view.
- RED HTTP regressions before the production change: the clean draft request returned HTTP 400 with the diagnosed missing-saved-view validation error, and the empty-view reset read back zero views instead of the canonical default.
- Implemented fresh backend `All tasks` defaults and presence-aware JSON overlay; explicit empty/null values remain explicit.
- Normalized explicit empty sidebar-view writes without an active field back to the canonical default, preserving the active/view referential invariant.
- The review regression was reproduced in `TestApplySidebarViews` and the HTTP reset path before normalization; both now pass.
- Added the real HTTP persistence regression, store coverage, canonical E2E reset state, and first-filter-edit browser scenario.
- All verification commands above pass; no blockers remain.
- PR #2293 was pushed as `b6ea4b50b` plus review-fix commit `cdc4f3737`. After the 15-minute fixup monitor, the current head reported 40 successful checks, zero failures, and zero unresolved review threads.
