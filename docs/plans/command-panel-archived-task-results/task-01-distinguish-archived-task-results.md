---
id: "01-distinguish-archived-task-results"
title: "Distinguish archived task results"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMMAND-PANEL-ARCHIVED-TASKS-001
acceptance_criteria:
  - AC-UI-COMMAND-PANEL-ARCHIVED-TASKS-001.1
  - AC-UI-COMMAND-PANEL-ARCHIVED-TASKS-001.2
  - AC-UI-COMMAND-PANEL-ARCHIVED-TASKS-001.3
  - AC-UI-COMMAND-PANEL-ARCHIVED-TASKS-001.4
  - AC-UI-COMMAND-PANEL-ARCHIVED-TASKS-001.5
  - AC-UI-COMMAND-PANEL-ARCHIVED-TASKS-001.6
  - AC-UI-COMMAND-PANEL-ARCHIVED-TASKS-001.7
  - AC-UI-COMMAND-PANEL-ARCHIVED-TASKS-001.8
system_design:
  - ../../specs/ui/system-design/command-panel-archived-task-results.md
---

# Task 01: Distinguish Archived Task Results

## Summary

Use authoritative archive state for command-panel search ordering and result presentation. Show archived status before the user opens a read-only task.

## In scope

- Replace terminal-state archive inference with `archived_at`.
- Page through archived search results before applying the command-panel display limit.
- Add the archived label, archive icon, accessible label, and muted semantic colors.
- Preserve non-archived activity icons, workflow badges, result selection, and navigation.
- Add focused hook, component, desktop, and phone tests.

## Out of scope

- Backend, archive-state, and task-detail changes.
- Search matching semantics or changes to the shared task-list API contract.
- Layout, scroll, or touch-target changes.

## Acceptance

- Archived results use an **Archived** badge, archive icon, accessible label, and muted semantic colors.
- Non-archived terminal tasks keep normal task presentation, and archived matches sort after non-archived matches even when the active match is on a later API page.
- Desktop and phone results remain readable, selectable, and free of document-level horizontal overflow.

## Verification

```bash
(
  cd apps/web
  pnpm exec vitest run hooks/use-command-panel-task-results.test.ts components/command-panel-task-activity.test.tsx
  pnpm run typecheck
  pnpm exec eslint --max-warnings 0 hooks/use-command-panel-task-results.ts components/command-panel-results.tsx hooks/use-command-panel-task-results.test.ts components/command-panel-task-activity.test.tsx
  pnpm run i18n:check
  pnpm e2e:run tests/command-panel.spec.ts -- --grep "archived task result"
  pnpm e2e:run --project mobile-chrome tests/search/mobile-command-palette-scopes.spec.ts -- --grep "archived task result"
)
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans apps/web
```

## Files likely touched

- `apps/web/hooks/use-command-panel-task-results.ts`
- `apps/web/hooks/use-command-panel-task-results.test.ts`
- `apps/web/components/command-panel-results.tsx`
- `apps/web/components/command-panel-task-activity.test.tsx`
- `apps/web/e2e/tests/command-panel.spec.ts`
- `apps/web/e2e/tests/search/mobile-command-palette-scopes.spec.ts`

## Dependencies

None. The task-list response and shared task-state icon already provide the required inputs.

## Risks

- A partial change can preserve incorrect archive ordering while fixing only the row appearance.
- Opacity on the complete row can make keyboard selection less clear.
- A second badge can reduce phone title space. The archived badge must replace the workflow-step badge.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-COMMAND-PANEL-ARCHIVED-TASKS-001`.
- `docs/specs/ui/system-design/command-panel-archived-task-results.md`.
- Existing command-panel search, activity-icon, and mobile-scope tests.

## Results

- RED: the hook and component regressions initially failed because terminal state was still the archive source and archived rows had no dedicated presentation.
- GREEN: `archived_at` now owns archive filtering, ordering, and row presentation. Archived rows use the archive icon, localized Archived label, replacement badge, muted semantic colors, and the existing selection callback.
- GREEN focused unit suite: 2 files, 16 tests passed.
- GREEN desktop E2E: the active result precedes the archived result, archive cues are visible, and selection navigates to the archived task detail.
- GREEN phone E2E: the same cues remain visible, title space remains usable, and document-level horizontal overflow is absent.
- GREEN typecheck, targeted ESLint, i18n checks, specification lint, and diff whitespace checks passed.
- Rendered evidence: disposable managed capture produced validated 1440x900 desktop and 393x852 phone PNGs; the capture spec was removed before staging.
- Review remediation: the hook now follows later task-list pages before applying the result limit; a regression covers archived first-page rows followed by an unarchived match. The mobile E2E fixture only seeds the archived task queried by that test.
