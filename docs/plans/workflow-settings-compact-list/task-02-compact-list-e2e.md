---
id: "02-compact-list-e2e"
title: "Compact workflow list E2E coverage and legacy spec compatibility"
status: done
wave: 2
depends_on:
  - "01-compact-list-ui"
plan: "plan.md"
requirements:
  - REQ-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001
  - REQ-UI-WORKFLOW-SETTINGS-COMPACT-LIST-002
acceptance_criteria:
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.1
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.3
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.4
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.6
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.7
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.8
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-002.2
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-002.4
system_design:
  - ../../specs/ui/system-design/workflow-settings-compact-list.md
---

# Task 02: Compact Workflow List E2E Coverage

## Summary and scope

Prove the user flow on desktop and phone, and keep existing workflow settings
E2E suites valid now that Compact is the default.

## Exclusions

No production changes except fixes for defects found here, which must be
recorded in Results. No unrelated spec rewrites.

## Acceptance

1. `e2e/tests/workflow/workflow-settings-compact-list.spec.ts` (chromium)
   covers the following. A fresh browser shows collapsed rows with step names
   and the view control on Compact. Expanding one row and renaming it shows the
   Unsaved marker after collapse, and the draft is intact after re-expansion.
   Saving persists the new name. Selecting Detailed and reloading keeps
   Detailed. Dragging a collapsed row reorders.
2. `e2e/tests/workflow/mobile-workflow-settings-compact-list.spec.ts`
   (mobile-chrome) covers the following. The row header and drag handle are at
   least 44 px tall. The document has no horizontal overflow. Tap expands and
   collapses. Detailed persists across reload.
3. `WorkflowSettingsPage.goto` seeds `detailed` with an init script only when
   no value is stored. The directly navigating specs listed in the plan's Risks
   pass. The whole `tests/workflow` directory passes on chromium.

## ASCII UI preview

See [plan UI-01 to UI-04](plan.md#ascii-ui-preview). The specs assert the
structure (rows, steps, markers, toggle placement, two-line phone row), not
pixel spacing.

## Verification

```bash
cd apps/web
pnpm e2e:run --project chromium tests/workflow/workflow-settings-compact-list.spec.ts
pnpm e2e:run --project mobile-chrome tests/workflow/mobile-workflow-settings-compact-list.spec.ts
pnpm e2e:run --project chromium tests/workflow
pnpm e2e:run --project chromium tests/improve-kandev.spec.ts tests/i18n/pseudo-coverage.spec.ts
pnpm e2e:run --project mobile-chrome tests/settings/mobile-workspace-clone.spec.ts \
  tests/workflow/mobile-queue-limit-navigation.spec.ts tests/workflow/mobile-workflow-settings.spec.ts
pnpm exec eslint --max-warnings 0 e2e/pages/workflow-settings-page.ts \
  e2e/tests/workflow/workflow-settings-compact-list.spec.ts \
  e2e/tests/workflow/mobile-workflow-settings-compact-list.spec.ts
```

Use `e2e/helpers/causal-waits.ts` for save waits. Do not use raw sleeps.

## Files and dependencies

- New: the two specs above.
- Changed: `apps/web/e2e/pages/workflow-settings-page.ts`, plus any directly
  navigating spec or helper that edits card fields (see the plan's Risks).
- Depends on work order 01 test IDs: `workflow-list-view-toggle`,
  `workflow-list-view-compact`, `workflow-list-view-detailed`,
  `workflow-compact-row-<id>`, and `data-expanded` on `workflow-card-<id>`.

Risks: hidden-but-mounted bodies can satisfy global locators, so scope them to
the card and require visibility.

## Results

- New specs pass: chromium `workflow-settings-compact-list.spec.ts` (2) and mobile-chrome
  `mobile-workflow-settings-compact-list.spec.ts` (1).
- `WorkflowSettingsPage.goto` seeds Detailed unless a view is stored (`listView: null` uses the
  default). `helpers/workspace-clone.ts` and the in-app-navigation prompt-hydration test in
  `workflow-settings.spec.ts` seed Detailed explicitly. Both WIP-queue navigation specs now also
  assert that the deep-linked card expands (`001.8`).
- The first full run caught test pollution: the compact spec renamed the shared seeded workflow.
  The spec now renames a workflow it creates.
- Final runs: chromium `tests/workflow`, `improve-kandev.spec.ts`, and `settings/workspace-clone.spec.ts`
  had 149 passed and 2 failed before the fixes. After the fixes, the affected specs passed (22 of 22 in
  one worker). mobile-chrome compact, workflow settings, WIP queue navigation, and workspace clone
  passed (12 of 12). Screenshots at desktop and phone widths matched the plan's UI previews.
- Test-phase feedback said the view control sat flush against the section divider (1 px gap). Both
  compact specs now assert a gap of at least 12 px through `listViewToggleGapBelowDivider`; this was
  RED at 1 px. The section children now have `pt-4`. The chromium compact, workflow settings, and
  sorting specs (24) and the mobile compact and workflow settings specs (8) pass. Screenshots with and
  without a sync config show 16 px spacing.
- Test-phase feedback said the sync status card and separator were indented 32 px (`pl-8`, left over
  from a removed drag-handle gutter) while still reaching the right edge. New desktop and mobile tests
  require the card and separator edges to match the workflow list within 1 px. A first version passed
  because it measured the padded wrapper; after switching to the rendered card and separator it was
  RED at 32 px. Removing `pl-8` fixed it. Chromium compact, workflow settings, and GitLab sync specs
  (26) pass, mobile compact and workflow settings specs (9) pass, and the sync-section unit tests (34)
  pass.
- PR review (Greptile and CodeRabbit) found three gaps:
  - A body shown in Detailed view now stays mounted after a switch to Compact. This was RED in the
    disclosure unit test.
  - A saved draft (new ID and React key) remounts expanded through `useIdsAddedAfterMount`. This was
    RED in E2E with `data-expanded="false"` after save.
  - Narrow fine-pointer windows now get 44 px row, drag-handle, and view-control targets (`max-md:`).
    This was RED in E2E at 32 px.

  AC 001.5 now excludes imported workflows, because import reloads the page. After the fixes, the
  chromium compact, sorting, workflow settings, and WIP-queue specs (29) and the mobile compact,
  workflow settings, and WIP-queue specs (10) pass, as do the focused Vitest suites (28 files, 258
  tests), `pnpm run typecheck`, oxlint, `i18n:ratchet`, and the spec validators.
