---
id: "01-repository-filter-none"
title: "Add and verify the None repository option"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-REPOSITORY-FILTER-NONE-001
acceptance_criteria:
  - AC-UI-REPOSITORY-FILTER-NONE-001.1
  - AC-UI-REPOSITORY-FILTER-NONE-001.2
  - AC-UI-REPOSITORY-FILTER-NONE-001.3
  - AC-UI-REPOSITORY-FILTER-NONE-001.4
  - AC-UI-REPOSITORY-FILTER-NONE-001.5
  - AC-UI-REPOSITORY-FILTER-NONE-001.6
  - AC-UI-REPOSITORY-FILTER-NONE-001.7
system_design:
  - ../../specs/ui/system-design/repository-filter-none.md
---

# Task 01: Add and Verify the None Repository Option

## Summary

Expose the existing missing-repository value in task-view repository pickers.
Complete the change with localized labels and tests of matching and restoration.

## In scope

- Prepend None in the repository branch of `useFilterValueOptions`.
- Reuse `task:none` and the existing persisted missing-value representation.
- Add hook, matching, conformance, and desktop/phone browser coverage.
- Update the selected-first E2E helper's expected repository options.
- Update the public task guide and its coverage evidence.

## Out of scope

New filter operators, storage or API changes, generic picker redesign,
named-repository matching changes, and other surfaces' option providers.

## Acceptance

1. The repository picker exposes one translated None option across the
   required catalog and locale states.
2. Positive, mixed, and negative selections use the existing matching and
   workspace-scoped persistence paths.
3. Desktop keyboard and phone touch flows pass the targeted checks, including
   existing selected-first regressions.

## UI preview

UI-01, from the [plan preview](plan.md#ui-preview), covers
AC-UI-REPOSITORY-FILTER-NONE-001.1, .3, and .7.

| View | Picker order | State |
| --- | --- | --- |
| Desktop | Search, None, kdlbs/kandev, remaining options | None and kdlbs/kandev selected |
| Phone | Filter drawer, value popover, search, scrolling options | Same selection with touch targets of at least 44px |
| Empty catalog | Search, None | None selectable |

Keep the search field above the option scroller and follow selected-first
ordering. Reuse existing surfaces, checked states, density, and motion.
The example repository and spacing are illustrative.

## Verification

Use `/tdd` and `/e2e`. First run the new hook/browser tests against the current
option provider to record the missing-option failure. Existing matcher tests
can pass before the UI change because matching already supports this value.

Run each command from the repository root. Install workspace dependencies once
with `(cd apps && pnpm install --frozen-lockfile)` if this worktree lacks them.

```bash
(cd apps/web && pnpm exec vitest run components/task/sidebar-filter/use-filter-value-options-repository.test.tsx components/task/sidebar-filter/use-filter-value-options.test.ts components/task/sidebar-filter/use-filter-value-options-locale.test.tsx lib/sidebar/apply-view.test.ts lib/sidebar/sidebar-local-conformance.test.ts components/task/sidebar-filter/use-filter-value-options-demand.test.tsx)
(cd apps/backend && go test ./internal/task/repository/sqlite -run '^TestSidebarLocalViewSQLiteConformance$' -count=1)
(cd apps/web && pnpm exec eslint components/task/sidebar-filter/use-filter-value-options.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/task/sidebar-repository-filter-none.spec.ts tests/task/sidebar-filter-selected-first.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-repository-filter-none.spec.ts tests/task/mobile-sidebar-filter-selected-first.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run desktop and phone suites sequentially. Managed E2E commands rebuild the
tested application. Record discovered tests, actual results, and the rendered
phone check. Add any new helper to the changed-file lint scope where required.

## Files likely touched

Production and hook coverage:

- `apps/web/components/task/sidebar-filter/use-filter-value-options.ts`
- `apps/web/components/task/sidebar-filter/use-filter-value-options-repository.test.tsx` (new)
- `apps/web/components/task/sidebar-filter/use-filter-value-options-locale.test.tsx`

Matching evidence:

- `apps/web/lib/sidebar/apply-view.test.ts`
- `apps/backend/internal/task/repository/testdata/sidebar-local-conformance.json`

Browser evidence:

- `apps/web/e2e/tests/task/sidebar-repository-filter-none.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-sidebar-repository-filter-none.spec.ts` (new)
- `apps/web/e2e/tests/task/sidebar-repository-filter-none-helpers.ts` (new, if needed)
- `apps/web/e2e/tests/task/sidebar-filter-selected-first-helpers.ts`

Delivery records:

- This work order, `plan.md`, and the paired requirement and system design.
- `docs/public/tasks-and-workflows.md`
- `docs/public/coverage.json`

## Dependencies

None.

## Risks

Retain the persisted token independently of translation. Use explicit empty
repository links in fixtures and assert the returned association state.
Existing selected-first assertions must include the extra option without
dropping their ordering, focus, or geometry checks.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/repository-filter-none.md)
- [System design](../../specs/ui/system-design/repository-filter-none.md)
- `use-filter-value-options-locale.test.tsx` for locale changes without store writes.
- `sidebar-filter-selected-first-helpers.ts` for real desktop/phone entry points.
- `sidebar-local-conformance.test.ts` and `sidebar_task_query_conformance_test.go`
  for the shared fixture contract.

## Results

Completed on 2026-10-19 after the user's explicit implementation request.

### TDD evidence

The new hook tests first failed because the option provider omitted None.
The desktop browser regression then failed at the missing None option.
The implementation adds the translated option in the repository branch only.
Existing matching and preference persistence remain the implementation path.

### Final checks

- The Vitest command above passed all 212 tests in six files. The demand-read
  suite also protects workflow metadata loading after the hook change.
- The targeted Go command passed `TestSidebarLocalViewSQLiteConformance`,
  including the four new positive and negative repository cases.
- ESLint passed the changed hook, hook tests, matching tests, new E2E files,
  and the existing selected-first helper. Prettier checks passed those files.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` passed. The implementation
  reuses the existing `task:none` key in every supported locale.
- `node --test scripts/validate-public-docs.test.mjs` passed 63 tests.
  `node scripts/validate-public-docs.mjs` validated 48 published pages.

### Browser commands and results

This host required a temporary child-process environment wrapper. It removes
inherited `KANDEV_*` runtime settings and supplies installed Nix browser
libraries through `LD_LIBRARY_PATH`. The wrapper and runtime setup remain
outside the repository at `/tmp/repository-filter-none-e2e-env.py`.

After the production build, the final test-only corrections reused that build:

```bash
python3 /tmp/repository-filter-none-e2e-env.py scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --no-build --project chromium tests/task/sidebar-repository-filter-none.spec.ts tests/task/sidebar-filter-selected-first.spec.ts
python3 /tmp/repository-filter-none-e2e-env.py scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --no-build --project mobile-chrome tests/task/mobile-sidebar-repository-filter-none.spec.ts tests/task/mobile-sidebar-filter-selected-first.spec.ts
```

Desktop passed two tests in 19.2 seconds. Log:
`/tmp/kandev-run.e2e.K4qlLy0m.log`.
Phone passed two tests in 32.9 seconds. Log:
`/tmp/kandev-run.e2e.c96mhwNb.log`.
Both managed runs used one worker and completed without test retries.

The phone assertions verified the drawer entry, a None target of at least
44px, picker containment with fractional pixel rounding, and document overflow.
The rendered screenshot confirmed readable options and the existing selected
indicator. No actionable UI polish findings remained in the inspected state.
Screenshot:
`apps/web/e2e/test-results/task-mobile-sidebar-reposi-4b1aa-hrough-the-task-view-picker-mobile-chrome/repository-none-phone.png`.

### Delivery record

The requirement is active, the design is current, and this plan is implemented.
Specification validation, coverage preflight, and diff checks passed.
PR publication follows the user's later explicit request.

### PR remediation

CI typecheck rejected the incomplete repository fixture cast. The fixture now
uses complete typed records and the shared branded-ID constructors. Review
feedback also added a comment for the existing missing-value representation
and an explicit search-fixture guard. The public behavior and design are
unchanged, so no requirement or system-design update is needed.

The six-file Vitest command passed all 212 tests after these changes.
Typecheck passed with CI's 8 GiB Node heap setting. Targeted ESLint and
Prettier checks passed for the three changed TypeScript files.

The desktop browser command above rebuilt the application and passed both
tests in 32.6 seconds (`/tmp/kandev-run.e2e.WcSCFaHQ.log`). The phone command
reused that build and passed both tests in 41.4 seconds
(`/tmp/kandev-run.e2e.wP3iNuW5.log`). Both runs completed without retries.
Remote checks and review resolution require fresh evidence after publication.

### Backend CI remediation

The retry launch fixture signalled provider acceptance before the executor
persisted the session's RUNNING state. A controlled startup barrier reproduced
CI's retained-failure assertion: expected `stopped`, actual `accepted`, with
the session still STARTING. The fixture now signals success from the executor's
process-started callback and asserts RUNNING before sending terminal events.

With the fixture repair applied to the failing CI run's merge commit, this
command passed all seven retry launch tests five times with the race detector
(35 runs):

```bash
(cd apps/backend && GOMAXPROCS=2 go test -trimpath -p 1 -race ./internal/orchestrator -run '^TestAutomationRetry' -count=5 -timeout=10m)
```

The run completed in 143.7 seconds. Changed-package Go lint reported zero
issues, and gofmt and diff checks passed. Remote CI and review evidence must
be refreshed after the remediation push.
