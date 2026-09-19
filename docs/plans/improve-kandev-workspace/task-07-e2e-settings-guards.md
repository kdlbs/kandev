# task-07: E2E settings guards for the dedicated workspace

Spec: `docs/specs/workspaces/requirements/improve-kandev.md` — "Dedicated workspace immutability"
Plan: `docs/plans/improve-kandev-workspace/plan.md` — Phase 2, Wave 7.
Depends on: task-06.

## Goal

Playwright evidence that the dedicated workspace's settings pages are
read-only and the backend rejects mutations, while a normal workspace stays
editable.

## Target

- `apps/web/e2e/tests/improve-kandev.spec.ts` (desktop; extend the existing
  improve-kandev suite).

## Change

Add tests (desktop chromium):

1. **Workflows settings read-only**: seed the dedicated workspace
   (`apiClient.createWorkspace("Improve Kandev")` + workflow via
   `createWorkflow`); `goto("/settings/workspace/<id>/workflows")`; assert:
   - The `improve-kandev` workflow card is listed with steps Improve → Test →
     PR (locate by card title / step text).
   - No "Add workflow" / "New workflow" control is present (assert the create
     control testid count is 0).
   - A read-only notice is visible.
   - (Optional) attempt a backend mutation via `apiClient` (e.g. create a
     workflow in the dedicated workspace) and assert it 409s — put the API
     rejection assertion here if the api client has a workflow-create helper,
     else rely on the integration tests.
2. **Repositories settings read-only**: with the dedicated workspace seeded
   (repo via `apiClient.createRepository`), `goto` the repositories settings
   page; assert the kandev repo card is listed and no add/delete controls are
   present.
3. **Sanity contrast**: the seed workspace's workflows page still shows the
   Add control (guards are workspace-scoped, not global).

## Acceptance

- `cd apps/web && pnpm build:vite && playwright test --config e2e/playwright.config.ts improve-kandev.spec` — all pass (desktop + mobile suites).
- No changes to unrelated e2e specs.

## Results

Covered by `improve-kandev.spec` tests "workflows settings are read-only in the
dedicated workspace" and "repositories settings are read-only in the dedicated
workspace": they seed workflows/repos under a temp workspace name, rename it to
"Improve Kandev" (rename is unguarded), then assert the settings pages render
read-only and that mutating API calls (create workflow, create repository) 409.
All pass in the 14/14 `improve-kandev.spec` run (see task-03).

> TODO(guards): workspace rename is intentionally unguarded (task-04 excludes
> it), so a user who can rename workspaces can bypass the immutability guard by
> renaming the dedicated workspace away, or trigger it by renaming any other
> workspace to "Improve Kandev". Track this gap when workspace-rename access is
> revisited; the name-based identity (`WorkspaceNameImproveKandev`) is the
> seam that would need to change (e.g. a dedicated flag column on the row).
