---
id: "03-build-repository-target-catalog"
title: "Build repository target catalog"
status: pending
wave: 3
depends_on:
  - "01-persist-automatic-color-settings"
  - "02-resolve-effective-task-colors"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003
acceptance_criteria:
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.1
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.2
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.3
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.4
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.6
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.7
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.8
system_design:
  - ../../specs/ui/system-design/sidebar-automatic-task-colors.md
---

# Task 03: Build Repository Target Catalog

## Summary

Build one repository catalog for automatic-color targets. Reuse current workspace, local, remote, and plugin discovery paths.

## In scope

- Stable workspace, provider, and local repository identities.
- Grouped options, duplicate removal, search, refresh, and unavailable stored targets.
- Generation guards and partial provider error state.
- A reusable view model for desktop and mobile pickers.
- Cross-workspace display rules for workspace, provider, and local targets.

## Out of scope

- Provider-specific API clients.
- Adding a discovered repository to a workspace.
- Final rule-card presentation.

## Acceptance

- One refresh reloads every source and ignores older results.
- Duplicate source records produce one stable option.
- Partial provider errors keep successful sources usable.
- Stored targets from another workspace remain visible and unavailable.

## Verification

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/integrations/use-remote-repositories.test.tsx components/task/add-workspace-sources/add-workspace-sources-dialog.test.tsx lib/sidebar/repository-rule-catalog.test.ts lib/sidebar/repository-rule-identity.test.ts)
```

## Files likely touched

- `apps/web/hooks/domains/integrations/use-remote-repositories.ts`
- `apps/web/hooks/domains/integrations/use-remote-repositories.test.tsx`
- `apps/web/hooks/domains/workspace/use-repositories.ts`
- `apps/web/lib/state/slices/workspace/workspace-slice.ts`
- `apps/web/components/task/add-workspace-sources/use-workspace-repository-options.ts`
- `apps/web/components/task/sidebar-filter/repository-rule-catalog.ts`
- `apps/web/components/task/sidebar-filter/repository-rule-catalog.test.tsx`

## Dependencies

Task 01 supplies the repository target wire type. Task 02 supplies identity primitives and matching behavior.

## Risks

- Remote provider identities can collide without host and scope fields.
- A plugin can return a repeated cursor or malformed record.

## Parallelism

`sequential`

## Inputs

- Repository identity and discovery sections in the system design.
- Existing task-creation and add-workspace-source repository pickers.

## Results

Implemented the grouped repository catalog and hook across workspace, local, built-in remote, plugin, and unavailable targets. Added stable identity keys, duplicate removal, query filtering, refresh generation, and provider-error preservation for the shared desktop/mobile picker.

Verification: repository catalog and identity tests passed as part of the 80-test focused frontend suite; the desktop and mobile Playwright flows exercised the picker and persisted target.
