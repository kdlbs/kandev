---
id: "01-workflow-step-grouping"
title: "Replace State grouping with workflow steps"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-LIST-STEP-GROUPING-001
acceptance_criteria:
  - AC-UI-LIST-STEP-GROUPING-001.1
  - AC-UI-LIST-STEP-GROUPING-001.2
  - AC-UI-LIST-STEP-GROUPING-001.3
  - AC-UI-LIST-STEP-GROUPING-001.4
  - AC-UI-LIST-STEP-GROUPING-001.5
  - AC-UI-LIST-STEP-GROUPING-001.6
  - AC-UI-LIST-STEP-GROUPING-001.7
system_design:
  - ../../specs/ui/system-design/task-list-workflow-step-grouping.md
---

# Task 01: Replace State Grouping with Workflow Steps

## Summary

Implement the [paired design](../../specs/ui/system-design/task-list-workflow-step-grouping.md)
as one vertical change. Both List surfaces shall use actual workflow steps,
with compatible settings and links, metadata recovery, and localized copy.

## In scope

- Canonical `workflow_step` grouping and legacy `state` normalization in backend
  settings and frontend parsers.
- Step metadata loading for the current result page, section projection, and
  shared desktop/phone control copy.
- Red/green tests, translations, public docs, and task-defined verification.

## Out of scope

- Sidebar, Threads, Office, task lifecycle, row-status icons, and page size or
  hierarchy changes.
- Creating persistent platform tasks/sessions. The user authorized commit,
  push, PR publication, and screenshots in the implementation request.

## Acceptance

1. Implement all seven referenced criteria using actual workflow/step IDs and
   configured step metadata, including cold All Workflows visits and recovery.
2. Preserve existing grouping values, hierarchy, and settings patch semantics;
   legacy State input resolves to the canonical new option and survives reload.
3. Desktop and phone rendered checks and the targeted verification commands
   pass, with translations and public help text matching the resulting UI.

## ASCII UI preview

The [full previews](plan.md#ascii-ui-preview) define UI-01 and UI-02.

```text
UI-01 desktop: Group [Workflow step v]
  BACKLOG: Task A + Task B (different runtime states)
  BUILD:   Task C (same runtime state as Task A)

UI-02 phone: List -> View options -> Task list
  Group
  [Workflow step                        v]
```

Criteria .1/.2/.7 apply. Retain the shipped phone drawer, 44px selector, safe
area, and internal scroller. Include workflow names in multi-workflow headings.
Preview spacing and task names are illustrative, not new UI data.

## Verification

Run from the repository root with Node 24 and pnpm on PATH. The current shell
can activate Node with `export PATH="$HOME/.nvm/versions/node/v24.18.0/bin:$PATH"`.
Install workspace dependencies once before the first package command in this
fresh worktree. Run typecheck before direct Vitest: its pretypecheck script
generates the ignored release-note and changelog JSON needed by SPA imports.
Follow `/tdd` and `/e2e`; first prove new scenarios fail against
the old implementation. Run E2E commands sequentially using the managed runner.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/user/...)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec vitest run lib/tasks/tasks-list-options.test.ts lib/ssr/user-settings.test.ts src/spa-routes.tasks-preferences.test.tsx app/tasks/tasks-list-view.test.tsx app/tasks/tasks-page-client.mr-hydration.test.tsx hooks/use-workflow-option-previews.test.ts hooks/use-task-list-workflow-steps.test.ts hooks/use-ensure-user-settings.test.ts lib/tasks/task-list-sections.test.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium tests/task/task-list.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/mobile-task-list-search.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

If a pure helper is extracted, add its adjacent test file to the exact Vitest
command before marking this work order done. Capture desktop and phone rendered
evidence during E2E and compare it with UI-01/UI-02. Record actual command
results and any blockers; do not test a stale Vite build.

## Files likely touched

- `apps/backend/internal/user/models/tasks_list_preferences.go`
- `apps/backend/internal/user/models/tasks_list_preferences_test.go` (new)
- `apps/backend/internal/user/service/service.go` and `service_test.go`
- `apps/backend/internal/user/store/sqlite_test.go`
- `apps/web/lib/tasks/tasks-list-options.ts` and `tasks-list-options.test.ts`
- `apps/web/lib/ssr/user-settings.test.ts`
- `apps/web/src/spa-routes.tasks-preferences.test.tsx` (new; use existing SPA route test patterns)
- `apps/web/hooks/use-workflow-option-previews.ts` and its test
- `apps/web/app/tasks/tasks-page-client.tsx`, `tasks-page-content.tsx`,
  `tasks-list-view.tsx`, and `tasks-list-view.test.tsx`
- `apps/web/components/kanban/mobile-menu-task-list-options.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,pseudo}/{tasks,kanban}.json`
- `apps/web/e2e/tests/task/task-list.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-list-search.spec.ts`
- `docs/public/tasks-and-workflows.md`
- The paired requirement/design and this plan/work order for lifecycle/results.

## Dependencies

None. Existing step-read APIs and the shared list display controls supply the
required boundaries. Read scoped guidance before changing backend or web code.

## Risks

- List Task DTOs supply placement IDs, not step names; metadata must load on a
  cold route and remain scoped to the selected workspace.
- Old settings/links and plugin facet selections must pass their respective
  parser paths without an accidental reset of other portable settings.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/task-list-workflow-step-grouping.md).
- [System design](../../specs/ui/system-design/task-list-workflow-step-grouping.md).
- Current shared option map, `useWorkflowOptionPreviews`, List grouping tests,
  and desktop/phone persistence E2E scenarios.

## Results

Implemented on 2026-10-01. Backend settings use canonical `workflow_step` values
while retaining `state` as accepted legacy input. Normal settings reads and
writes normalize old JSON without a database migration or revision change.

The List projection groups by workflow/step IDs, resolves configured names and
positions from scoped step-only reads, keeps hierarchy and row sorting, and
retains rows through loading or failed metadata. Refresh, reconnection, and
relevant step notifications invalidate metadata generations, including on cold
All Workflows visits. Desktop and phone controls share the localized choice.
The public task-list documentation explains the new choice and old-link behavior.

Red assertions reproduced old default/alias behavior, runtime-state section
splitting, missing refresh generation invalidation, and the cold-list step-event
gap before their production fixes. Green results:

- `go test ./internal/user/...`: all six packages passed.
- The targeted nine-file Vitest command passed 116 tests; the updated five-test
  metadata-hook suite then passed the additional combined-refresh scenario.
- Typecheck, changed-file ESLint (zero warnings), localization generation,
  `i18n:check`, and `i18n:ratchet`: passed.
- Desktop managed E2E: six passed; mobile-chrome managed E2E: two passed. Tests
  cover old links, portable persistence, cold workflows, live step renaming,
  hierarchy, sorting, search, archived rows, pagination, and phone geometry.
- Public docs: 62 validator tests and 47 published pages passed. Spec catalog,
  36 specification-linter tests, all-spec lint, documentation coverage preflight,
  and `git diff --check`: passed.

UI-01/UI-02 rendered checks show the Workflow step choice, configured headings,
and separate workflow sections. Three fresh PNGs were inspected and compressed
under ignored `.pr-assets`: desktop groups, phone groups, and phone View options.
Phone controls retain their 44px selector and fit without horizontal overflow.
PR screenshot media is published separately from the implementation branch.

The first integrated live-rename test used the wrong raw-response accessor and
HTTP method; both test-harness mistakes were corrected to the existing native
Response contract and PUT route before the desktop suite passed. The final
runs reused the freshly built unchanged production artifacts. No unresolved
implementation or validation blockers remain.

PR review exposed stale initial workflow data after workspace changes or
workflow creation. The List now projects authorized IDs, names, and ordering
from the current workspace store. Two new regression tests reproduced these
gaps before the fix; the metadata suite now passes seven tests. Four affected
frontend suites passed 17 tests, and typecheck, ESLint, and localization checks
passed. The desktop Select also uses intrinsic width with a 150px minimum:
the Portuguese E2E test reproduced clipping and now passes. Seven desktop E2E
tests and two phone E2E tests passed, including foreground refresh resolving
a newly created workflow's configured step.
Four refreshed screenshots cover English and Portuguese desktop groups, phone
groups, and phone View options; all were inspected and compressed from synthetic
test data for publication in the PR.
