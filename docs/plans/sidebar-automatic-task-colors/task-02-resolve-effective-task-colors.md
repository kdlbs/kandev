---
id: "02-resolve-effective-task-colors"
title: "Resolve effective task colors"
status: done
wave: 2
depends_on:
  - "01-persist-automatic-color-settings"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003
acceptance_criteria:
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.2
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.3
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.4
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.5
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.6
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.7
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.8
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.9
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.10
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.11
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.12
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.14
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.15
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.5
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.7
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.8
system_design:
  - ../../specs/ui/system-design/sidebar-automatic-task-colors.md
---

# Task 02: Resolve Effective Task Colors

## Summary

Add the pure first-match rule engine and connect it to sidebar task rows. Preserve manual colors as the fallback source.

## In scope

- Rule normalization and matching for all seven dimensions.
- Shared option builders and explicit differences from sidebar filter dimensions.
- Separate manual, fixed-rule, and effective color presentation types.
- Safe workflow-step color mapping with a gray fallback.
- Desktop, mobile, and archived task fact projection with live recalculation.
- Pure repository identity matching for workspace, provider, and local targets.
- Repository metadata joins at each task projection boundary.
- Automatic-source information in the existing task color menu.

## Out of scope

- Repository option discovery and picker UI.
- Rule editing controls.
- Changes to task or workflow persistence.

## Acceptance

- The first enabled match wins for every supported dimension.
- Automatic output hides but never erases the manual value.
- Task and workflow updates change the marker without a task-color write.
- Missing origins normalize to Kanban. Missing task facts do not match their dimensions.
- The manual menu remains seven colors while fixed automatic rules support ten colors.

## Verification

```bash
(cd apps/web && pnpm exec vitest run lib/task-colors.test.ts lib/task-color-presentation.test.ts lib/sidebar/repository-rule-identity.test.ts lib/sidebar/task-color-rules.test.ts components/task/sidebar-filter/use-filter-value-options.test.ts components/task/sidebar-filter/task-color-rule-options.test.tsx components/task/task-session-sidebar-item.test.ts components/task/mobile/session-task-switcher-sheet-item.test.ts components/task/task-session-sidebar-archived-item.test.ts components/task/task-item.test.tsx components/task/task-switcher-context-menu.test.tsx)
```

## Files likely touched

- `apps/web/lib/task-colors.ts`
- `apps/web/lib/task-color-presentation.ts`
- `apps/web/lib/task-color-presentation.test.ts`
- `apps/web/lib/sidebar/sidebar-dimension-options.ts`
- `apps/web/lib/sidebar/repository-rule-identity.ts`
- `apps/web/lib/sidebar/repository-rule-identity.test.ts`
- `apps/web/lib/sidebar/task-color-rules.ts`
- `apps/web/lib/sidebar/task-color-rules.test.ts`
- `apps/web/components/task/sidebar-filter/use-filter-value-options.ts`
- `apps/web/components/task/sidebar-filter/task-color-rule-options.ts`
- `apps/web/components/task/sidebar-filter/task-color-rule-options.test.tsx`
- `apps/web/components/settings/workflow-pipeline-editor-helpers.tsx`
- `apps/web/components/task/task-switcher-types.ts`
- `apps/web/components/task/task-session-sidebar-item.ts`
- `apps/web/components/task/task-session-sidebar.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet-item.ts`
- `apps/web/components/task/task-session-sidebar-archived-item.ts`
- `apps/web/components/task/task-switcher-row.tsx`
- `apps/web/components/task/task-item.tsx`
- `apps/web/components/task/task-switcher-color-menu.tsx`

## Dependencies

Task 01 supplies normalized portable rules and repository target wire types.

## Risks

- Per-row subscriptions can cause broad rerenders. Derive automatic colors at the sidebar projection boundary.
- Workflow classes must map through a shared safe presentation parser.
- Omitted projection tests can let desktop, mobile, and archived behavior drift.

## Parallelism

`sequential`

## Inputs

- Rule model and color-resolution sections in the system design.
- Existing manual color store and task-row projection.

## Results

Implemented first-match resolution, ten-color presentation, workflow-step color parsing, repository identity matching, desktop/mobile task facts, and automatic-source disclosure in the manual color menu. Manual colors remain device-local and are used when no automatic rule matches.

Verification: focused resolver, presentation, identity, projection, option, mapper, and sidebar suites passed 80 tests. Desktop and mobile Playwright flows passed with live state changes and reload persistence.
