---
created: 2026-08-31
status: implemented
requirements:
  - REQ-UI-COMMAND-PANEL-TASK-ACTIVITY-001
system_design:
  - ../../specs/ui/system-design/command-panel-task-activity-icons.md
legacy_specs: []
---

# Implementation Plan: Command Panel Task Activity Icons

## Overview

Share the sidebar task-state icon with command-panel task results. Then connect each result to the newest bounded task projection.

One work order owns the change because the shared icon, result row, and focused tests form one vertical result.

## Scope

### In scope

- Extract the sidebar task-state icon into a shared presentation component.
- Use the shared component in sidebar and command-panel task rows.
- Select the newest HTTP or live status summary for each command-panel result.
- Keep the current workflow-step badge and row navigation.
- Add desktop, phone, and component evidence.

### Out of scope

- Change task or session state rules.
- Change backend response fields or WebSocket events.
- Change task search ranking, limits, or navigation.
- Change command-panel geometry, touch behavior, or scroll ownership.

## Technical approach

### Shared task-state icon

Extract `TaskStateIcon` and its small support functions from `components/task/task-item.tsx`. Put the shared presentation in a task UI module.

Keep the sidebar icon priority and data attributes. Add size and placement inputs so each surface keeps its current row density.

### Command-panel live state

Read live task projections from the current Kanban snapshots. Match each projection by task ID.

Use `pickFreshestStatusSummary` to compare the result summary and the live summary. Use lifecycle fields from the newer task projection.

Map snake-case HTTP fields and camel-case store fields into the shared icon input. Derive the final step ID per workflow from the visible Kanban step list and pass it through to the shared icon so review tasks keep sidebar completion parity. Do not subscribe to session-detail streams.

An accepted live projection is authoritative for lifecycle and foreground-activity fields, including an explicit clear. Legacy projections without `updatedAt` are treated as current so a live clear cannot fall back to an older search response.

### Result-row presentation

Replace the archive or hammer glyph in the command panel with the shared task-state icon. Keep the other result fields unchanged.

Add an accessible state description to the icon. Keep the complete option as the only action and keyboard target.

### Mobile design note

Desktop and phone layouts use the current command-panel result row. The nearest phone example is `mobile-command-palette-scopes.spec.ts`.

This change replaces one content icon. It does not change composition, navigation, scrolling, safe areas, or touch targets.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.1` to `.3` | Component tests compare running and idle command-panel icons with sidebar icon states. |
| `AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.4` | A component test changes the live summary revision after the result renders. |
| `AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.5` and `.7` | Component tests preserve the badge, option semantics, selection, and accessible state text. |
| `AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.6` | Focused desktop and phone Playwright tests assert the icon and title width. |

## E2E tests

Update `apps/web/e2e/tests/command-panel.spec.ts`. Add a running task and an idle task, then assert their command-panel icon states.

Update `apps/web/e2e/tests/search/mobile-command-palette-scopes.spec.ts`. Assert the active task icon and preserve the existing title-width check.

## Work orders

- [x] [Task 01: Share task activity icons](task-01-share-task-activity-icons.md)

## Verification results

- Shared component tests: 88 passed.
- TypeScript, targeted ESLint, and full i18n validation passed.
- Desktop Chromium task-icon E2E: 1 passed.
- Mobile Chrome task-icon and layout E2E: 1 passed.
- Specification lint and diff checks passed.

## Risks

- An HTTP result can race with a WebSocket update. Summary revision comparison must reject the older result.
- The sidebar has more task-state variants than the current command panel. Shared logic must preserve all sidebar priorities.
- Review tasks need the workflow's final step ID to retain the sidebar's completed-versus-turn-finished distinction.
- Live projections can explicitly clear foreground activity; the merge must preserve that clear instead of reviving stale HTTP activity.
- The leading icon must not reduce title width on a phone.
