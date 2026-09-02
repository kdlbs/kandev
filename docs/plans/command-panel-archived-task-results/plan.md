---
created: 2026-09-02
status: complete
requirements:
  - REQ-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001
system_design:
  - ../../specs/tasks/system-design/command-panel-archived-task-results.md
legacy_specs: []
---

# Implementation Plan: Command Panel Archived Task Results

## Overview

Use `archived_at` for command-panel archive classification. Then give archived task results a distinct label, icon, and muted color.

One work order owns this vertical UI change and its focused desktop and phone evidence.

## Scope

### In scope

- Classify and order archived task results by `archived_at`.
- Replace the activity icon with an archive icon for archived results.
- Replace the workflow-step badge with an **Archived** badge for archived results.
- Use muted semantic colors without weakening selection feedback.
- Add hook, component, desktop, and phone regression coverage.

### Out of scope

- Backend, API, archive-state, or task-detail changes.
- Search matching semantics or changes to the shared task-list API contract.
- Command-panel geometry, scrolling, or touch-target changes.
- Changes to task-state icons on other surfaces.

## Technical approach

### Archive classification and ordering

Update `apps/web/hooks/use-command-panel-task-results.ts`. Use `task.archived_at == null` for the unarchived preview and `task.archived_at != null` for search-result grouping.

Request page one of the default unarchived search results with the display limit. If fewer rows are returned, request only archived matches with the remaining limit and concatenate the two groups. Non-archived matches remain first, backend order stays intact within each group, and the search performs at most two requests.

Add hook cases for an archived in-progress task and an unarchived terminal task.

### Result-row presentation

Update `TaskResultItem` in `apps/web/components/command-panel-results.tsx`. Derive archive presentation from `task.archived_at`.

For an archived result, show `IconArchive` with a localized accessible label. Replace the workflow-step badge with an outline **Archived** badge.

Use muted semantic text and border classes. Do not apply opacity to the command item because it also mutes selection feedback.

Keep `TaskStateIcon`, workflow-step color, and current activity resolution for non-archived results. Keep the current selection callback for every result.

### Localization

Reuse `tasks:archived`. The key exists in all supported locale catalogs, so the implementation does not add user-facing copy.

### Mobile parity

Desktop and phone use the same existing result row. The nearest mobile example is `apps/web/e2e/tests/search/mobile-command-palette-scopes.spec.ts`.

This change replaces one icon and one badge. It does not change composition, navigation, scrolling, safe areas, or touch targets.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.1` to `.4` | Component tests cover the badge, icon, accessible label, and muted semantic classes. |
| `AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.5` and `.6` | Hook tests prove `archived_at` classification and ordering, including bounded active and archived requests. |
| `AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.7` | A component test invokes the existing selection callback for an archived result. |
| `AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.8` | Desktop and phone Playwright tests cover visible cues, title space, and overflow. |

## E2E tests

Extend `apps/web/e2e/tests/command-panel.spec.ts` with active and archived matches. Assert the non-archived result comes first and the archived result shows its label and icon.

Extend `apps/web/e2e/tests/search/mobile-command-palette-scopes.spec.ts`. Assert the same archive cues and no document-level horizontal overflow.

## Work orders

- [x] [Task 01: Distinguish archived task results](task-01-distinguish-archived-task-results.md) - complete; focused unit, desktop, and phone evidence passes

## Verification results

- GREEN focused unit suite: `cd apps/web && pnpm exec vitest run hooks/use-command-panel-task-results.test.ts components/command-panel-task-activity.test.tsx` - 2 files, 16 tests passed.
- GREEN typecheck: `cd apps/web && pnpm run typecheck` - passed.
- GREEN targeted ESLint: `cd apps/web && pnpm exec eslint --max-warnings 0 hooks/use-command-panel-task-results.ts components/command-panel-results.tsx hooks/use-command-panel-task-results.test.ts components/command-panel-task-activity.test.tsx` - passed.
- GREEN i18n checks: `cd apps/web && pnpm run i18n:check` - all catalogs and copy guards passed.
- GREEN desktop E2E: `cd apps/web && pnpm e2e:run tests/command-panel.spec.ts -- --grep "archived task result" --retries=0` - 1 test passed.
- GREEN phone E2E: `cd apps/web && pnpm e2e:run --project mobile-chrome tests/search/mobile-command-palette-scopes.spec.ts -- --grep "archived task result" --retries=0` - 1 test passed.
- GREEN specification lint: `python3 scripts/lint-spec-files.py --all` - passed.
- GREEN whitespace check: `git diff --check -- docs/specs docs/plans apps/web` - passed.
- GREEN rendered evidence: disposable managed E2E capture produced validated 1440x900 desktop and 393x852 phone PNGs; the temporary capture spec was removed and the assets remain ignored for PR publication.
- Review remediation: search results now use a bounded active-then-archived fallback through the existing task-list API. A regression covers a large archived total without issuing more than two requests.

## Risks

- The task-list API returns one archive mode per request. The hook must query unarchived matches before using an archived fallback so archive ordering is correct without an unbounded request waterfall.
- Row opacity can hide the selected state. The implementation must use semantic muted colors instead.
- The archived badge must replace the workflow-step badge so phone title space does not decrease.
