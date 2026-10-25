---
id: "04-e2e"
title: "E2E: multi-repository automation editor flows"
status: pending
wave: 3
depends_on: ["02-orchestrator-resolution", "03-frontend-multi-repo-picker"]
plan: "plan.md"
spec: "../../specs/office/requirements/automations-settings.md"
---

# Task 04: E2E coverage for multi-repository automation editing

## Inputs

- `docs/specs/office/requirements/automations-settings.md` — Scenarios section (executor
  gating + repeatable list rendering).
- `apps/web/e2e/README.md` — project/tag conventions (which Playwright
  project this belongs in; this feature has no Docker/SSH dependency, so it
  should run in the default project, not the `containers` project).
- Existing automation editor E2E specs, if any — search
  `apps/web/e2e/**/automation*` first and follow the same fixtures/selectors
  (workspace seeding, `data-testid="repository-selector"` /
  `data-testid="execution-mode-selector"` established in
  `config-section.tsx`).
- Task 03's new `data-testid`s for the repeatable row list and "Add
  repository" button (name them there; reference the exact test ids chosen).

## Change

Add `apps/web/e2e/automations/multi-repository.spec.ts` with:

1. **Compatible executor → repeatable list.** Seed a workspace with a
   `worktree` executor profile and two repositories. Open the automation
   editor (new automation), select the worktree executor profile, verify
   "Add repository" is visible/enabled, add a second repository row, select
   both repositories, save, reload the editor, and assert both repositories
   are pre-selected.
2. **Incompatible executor → single dropdown.** Same workspace, select a
   `local` executor profile, open the repository picker, assert it renders as
   a single dropdown and no "Add repository" control exists in the DOM.
3. **Executor picker gating.** With two repositories already selected (via
   step 1's setup or a fresh compatible-executor selection), open the
   Executor Profile dropdown and assert the `local` (or other incompatible)
   option is disabled/not selectable.

## Acceptance

- All three scenarios pass headlessly in the default Playwright project
  (`pnpm e2e`), no `KANDEV_E2E_CONTAINERS` dependency.
- Specs follow existing automation-editor E2E conventions for workspace/data
  seeding (reuse existing fixtures rather than hand-rolling new seed logic).

## Verification

```
cd apps/web && pnpm e2e -- automations/multi-repository.spec.ts
```

## Files likely touched

- `apps/web/e2e/automations/multi-repository.spec.ts` (new)

## Dependencies

Tasks 02 and 03 (needs both the backend resolution behavior and the frontend
picker to exist).

## Parallelism

`sequential` (wave 3, last).

## Output contract

Summary of changes, exact `pnpm e2e` command output for the new spec file,
and a note updating `plan.md`'s Wave 3 checkbox and this file's `status` to
`done`.
