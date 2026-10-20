---
created: 2026-10-19
status: implemented
requirements:
  - REQ-UI-REPOSITORY-FILTER-NONE-001
system_design:
  - ../../specs/ui/system-design/repository-filter-none.md
legacy_specs: []
---

# Implementation Plan: Repository Filter None

## Overview

Add None to task-view repository values in one vertical work order.
The existing missing-value representation supports matching and persistence.
The implementation adds its selectable label and tests the complete flow.

## Scope

### In scope

- Add the option to single-value and multi-value repository clauses.
- Preserve matching, localization, and workspace-scoped saved views.
- Prove desktop and phone behavior with targeted tests.
- Document None and mixed selections in the public task guide.

### Out of scope

Board display selectors, Threads, automatic-color rules, repository identity
redesign, and membership writes.

## Technical approach

Prepend the missing-value option in `useFilterValueOptions` with value
`"undefined"` and the existing `task:none` translation. Keep `repositoryOptions`
and generic picker rendering unchanged. Verify matching through
`applyFilters`, the local sidebar projection, and SQL conformance fixtures.

Use the current workspace-scoped draft/save path. The change needs no schema
or API expansion. Update the selected-first E2E helper's expected option set
because the hook now supplies one additional value.

## UI preview

UI-01 shows the repository value picker after selecting None and one named
repository. Labels use the active locale. The repository names are illustrative.

| View | Entry point | Control order | Selection |
| --- | --- | --- | --- |
| UI-01 desktop | Tasks filter gear, Repository, In list, values | Search, None, kdlbs/kandev, other repositories | None and kdlbs/kandev selected |
| UI-01 phone | Task picker, filter gear, Repository values | Drawer editor, searchable value popover, scrolling options | Same selected values |
| UI-01 empty catalog | Either editor, Repository values | Search, None | None remains selectable |

Search stays above the options. Selected rows follow the shared selected-first
contract. The option list owns scrolling. The phone drawer retains safe-area
padding, and option targets remain at least 44px. Existing dismissal and focus
return apply. These structures are required. Spacing and example names are
illustrative. This preview covers AC-UI-REPOSITORY-FILTER-NONE-001.1, .3, and .7.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `.1`, `.6` | New `use-filter-value-options-repository.test.tsx` and existing `use-filter-value-options-locale.test.tsx` |
| `.2`, `.3`, `.4` | `apply-view.test.ts`, shared `sidebar-local-conformance.json`, and both conformance runners |
| `.5` | Save, reload, and workspace-return assertions in the focused E2E flow |
| `.7` | Desktop keyboard and phone touch E2E, including option geometry |

All abbreviated criteria above belong to `AC-UI-REPOSITORY-FILTER-NONE-001`.
Keep fixtures explicit about empty repository links. Production task creation
accepts `repository_ids: []` through the E2E API helper. Check the created
task's returned repositories before using it as the None fixture.

## E2E tests

Add `apps/web/e2e/tests/task/sidebar-repository-filter-none.spec.ts` for Chromium
and `mobile-sidebar-repository-filter-none.spec.ts` for `mobile-chrome`.
Reuse a focused helper for seeding and selection assertions.

Seed a task with no repository, a task in repository A, and a task in repository B.
Prove None-only, None plus A, removal, and save/reload through visible task results.
Exercise a second workspace and return to the first saved view.
Existing filters remain in the flow so conjunction behavior receives evidence.

On desktop, select None through the real keyboard path from the visible opener.
On phone, use `.tap()`, check the drawer entry, measure the None row, and inspect
containment, scrolling, and horizontal overflow. Capture the focused phone state.

Run the existing selected-first specs with the new specs after updating their
expected repository option set. Preserve their existing checks.

## Work orders

- [x] [Task 01: Add and verify the None repository option](task-01-repository-filter-none.md)

Execute sequentially. The work order contains the exact verification commands.

## Verification results

Implementation completed on 2026-10-19. Work order 01 passed its targeted
checks: 212 frontend tests, SQLite conformance, two desktop E2E tests, and two
phone E2E tests. Lint, formatting, translation, and public-doc checks passed.
The [work order results](task-01-repository-filter-none.md#results) record the
commands, local runtime setup, and browser evidence.

PR remediation completed locally: repository test fixtures now satisfy their
types, the existing missing-value representation has an inline explanation,
and the selected-first helper checks its search precondition. Typecheck,
212 focused tests, two desktop browser tests, two phone browser tests, and
targeted lint and formatting passed. The
[remediation record](task-01-repository-filter-none.md#pr-remediation) records
the scope and results. Remote verification remains pending until the published
head passes its checks.

Public docs updated: `docs/public/tasks-and-workflows.md` (how-to guidance),
with source and browser-test evidence in `docs/public/coverage.json`.

Design validation on 2026-10-19:

- `python3 scripts/list-docs.py validate`: Passed. Validated 384 decisions and
  1,618 specifications.
- `python3 scripts/lint-spec-files.test.py`: Passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: Passed.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage` preflight: Covered.
  The check used the four draft documents and the planned production path.
- `git diff --check -- docs/specs docs/plans/repository-filter-none`: Passed.
  Direct checks also confirmed newline and whitespace validity in all four
  untracked documents. Catalog discovery and status checks confirmed all files.

## Risks

- Existing missing-metadata and literal `undefined` slug ambiguity follow the
  current filter representation. This work does not introduce a new matcher.
- Browser fixtures that assert the repository option count need the synthetic
  row in their expected set.
