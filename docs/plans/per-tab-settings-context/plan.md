---
created: 2026-10-04
status: complete
requirements:
  - REQ-WORKSPACES-PER-TAB-SETTINGS-CONTEXT-001
system_design:
  - ../../specs/workspaces/system-design/per-tab-settings-context.md
legacy_specs: []
---

# Implementation Plan: Per-tab Settings Workspace Context

## Overview

Keep a valid tab-local workspace active when a user opens Settings. The
implementation uses the existing workspace list, shared cookie, saved
preference, and Zustand state. [Task 01](task-01-per-tab-settings-context.md)
records the completed implementation and its coverage.

## Scope

### In scope

- Preserve the valid workspace that is active in the current tab.
- Use the shared cookie, saved preference, first workspace, and no selection as
  ordered fallbacks.
- Keep Office and Kanban workspaces eligible.
- Preserve active-workspace revision updates during hydration.

### Out of scope

- Changing the cookie or saved preference contract.
- Adding backend APIs, persisted fields, or workspace type filtering.

## Technical approach

`loadSettingsInitialState` loads workspace and user-settings data. The Settings
bootstrap reads the current tab's workspace after those requests complete and
passes it to `resolveSettingsActiveWorkspaceId`. The resolver accepts only
workspace IDs present in the loaded list. Hydration keeps the previous active
ID until `setActiveWorkspace` applies a changed selection and updates the
revision used by workspace-scoped consumers.

The active selection belongs to the frontend workspace store. The backend
continues to own the workspace list and saved preference. The shared cookie
remains a fallback for tabs without a valid local selection.

## Tests

| Criteria | Evidence |
| --- | --- |
| `.1`-`.4` | `apps/web/lib/routing/route-bootstrap.test.ts`: `resolveSettingsActiveWorkspaceId` candidate precedence, validation, workspace-type eligibility, and empty-list cases. |
| `.1`-`.5` | `apps/web/src/settings-routes.test.ts`: `buildSettingsInitialStateForRoute` current-tab and fallback hydration. |
| `.1` | `apps/web/src/settings-routes.workspace-revision.test.tsx`: current-tab selection and revision updates through `SettingsRouteBootstrap`. |
| `.1`, `.5` | `apps/web/e2e/tests/settings/workspace-tab-settings-home.spec.ts` and `mobile-workspace-tab-settings-home.spec.ts`: desktop and phone navigation from Settings to Home. |

## E2E tests

The desktop and phone scenarios verify that each tab returns Home with its own
active workspace after Settings navigation. See the two Settings E2E files
listed above.

## Work orders

- [x] [Task 01: Preserve per-tab Settings workspace context](task-01-per-tab-settings-context.md)

## Verification results

The implementation was committed in PR #4199 at
`2854711f9d360ff1fa66fd5f6635963be736ec23`. Focused unit tests passed (44
tests). The desktop and phone E2E cases passed. Focused ESLint and commit hooks
passed. The local documentation coverage preflight, catalog validation, and
specification linters passed. GitHub will rerun the PR coverage check after
this work order is pushed.

## Risks

- The shared cookie can change when another tab selects a workspace. The
  current tab therefore needs to retain its own valid active ID during
  Settings hydration.
