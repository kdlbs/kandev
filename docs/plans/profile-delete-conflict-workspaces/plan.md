---
created: 2026-11-03
status: implemented
requirements:
  - REQ-AGENTS-PROFILE-DELETE-WORKSPACES-001
system_design:
  - ../../specs/agents/system-design/profile-delete-conflict-workspaces.md
legacy_specs: []
---

# Implementation Plan: Group profile delete conflicts by workspace

## Overview

Add workspace identity to the profile delete-conflict `active_sessions` rows
and group the conflict dialog's Tasks and Quick Chats sections by workspace.
One vertical work order: the backend field and the dialog grouping are only
verifiable together end to end, and the change is small.

## Scope

### In scope

- `workspace_id` and `workspace_name` on `ActiveTaskInfo` and the 409 body.
- Workspace-grouped Tasks and Quick Chats sections in the conflict dialog.
- New `agents:conflictUnknownWorkspace` copy in all locales.
- Go repository test, Vitest helper and component tests, Playwright assertion.

### Out of scope

- Links to tasks, other conflict sections, Office agent deletion, and the
  blocking-session predicate.

## Technical approach

Backend:

- `apps/backend/internal/agent/dto/dto.go`: add
  `WorkspaceID string \`json:"workspace_id"\`` and
  `WorkspaceName string \`json:"workspace_name"\`` to `ActiveTaskInfo`.
- `apps/backend/internal/task/repository/sqlite/session.go`
  `GetActiveTaskInfoByAgentProfile`: select
  `t.workspace_id, COALESCE(w.name, '')` with
  `LEFT JOIN workspaces w ON w.id = t.workspace_id`; scan the new fields.
  Keep the existing filter. Ordering stays a client concern.

Frontend:

- `apps/web/lib/types/agent-profile-errors.ts`: optional `workspace_id?` and
  `workspace_name?` on `ActiveSessionInfo`.
- New `apps/web/lib/settings/agent-profile-delete-conflict.ts` exporting
  `groupSessionsByWorkspace(sessions: ActiveSessionInfo[])` returning
  `{ workspaceId: string; workspaceName: string; sessions: ActiveSessionInfo[] }[]`,
  sorted per AC .3 with `localeCompare(..., { sensitivity: "base" })` and ID
  tie-breaks. Groups with an empty name sort after named groups.
- `apps/web/components/settings/agent-profile-delete-dialog.tsx`:
  `SessionConflictSection` takes `workspaceLabels: Map<string, string>` (the
  map already built for automations) and renders groups with heading label
  `workspaceName || workspaceLabels.get(workspaceId) || t("agents:conflictUnknownWorkspace")`.
  Extract the shared `workspaceLabels` map once in the dialog.
- Locales: add `conflictUnknownWorkspace` to `src/locales/<locale>/agents.json`
  for `en`, `pt-pt`, `zh-cn`, `ja`, `ko`, and the pseudo catalog; run
  `pnpm run i18n:zh-hant` for `zh-hk`/`zh-tw` and `pnpm run i18n:pseudo` if the
  pseudo catalog is generated.

## ASCII UI preview

`UI-01: Profile delete conflict dialog, tasks in two workspaces` (entry point:
profile settings page, Delete profile, confirm, 409 conflict). Structural
requirements: workspace headings sit inside each section, below the section
title; items are indented beneath their heading; the body scrolls and the
footer stays fixed. Spacing and styling are illustrative.

Before:

```text
+--------------------------------------------------+
| Delete agent profile                             |
| This profile is currently in use. Deleting it    |
| will affect the following:                       |
| Tasks:                                           |
|  - Fix login                                     |
|  - Add billing page                              |
|  - Refactor auth                                 |
| ...consequences...                               |
|                        [Cancel] [Delete anyway]  |
+--------------------------------------------------+
```

After (desktop):

```text
+--------------------------------------------------+
| Delete agent profile                             |
| +----------------------------------------------+ |
| | This profile is currently in use. Deleting   | |  <- scrolls
| | it will affect the following:                | |
| | Tasks:                                       | |
| |   Backend                                    | |
| |    - Fix login                               | |
| |    - Refactor auth                           | |
| |   Frontend                                   | |
| |    - Add billing page                        | |
| | Quick Chats:                                 | |
| |   Unknown workspace                          | |
| |    - Untitled quick chat                     | |
| | ...consequences...                           | |
| +----------------------------------------------+ |
|                        [Cancel] [Delete anyway]  |  <- fixed
+--------------------------------------------------+
```

Phone: same composition inside the contained dialog; footer actions stack
full width (existing behavior), long workspace names wrap.

```text
+--------------------------+
| Delete agent profile     |
| Tasks:                   |
|   Backend                |
|    - Fix login           |
|   A very long workspace  |
|   name that wraps        |
|    - Add billing page    |
| ...                      |
| [        Cancel        ] |
| [    Delete anyway     ] |
+--------------------------+
```

Maps to AC .1, .2, .4, .5.

## Tests

| AC | Evidence |
| --- | --- |
| .1, .2 | `apps/backend/internal/task/repository/sqlite/agent_profile_blockers_workspace_test.go` `TestAgentProfileBlockersIncludeWorkspace` (rows carry workspace ID and name); `agent-profile-delete-dialog.test.tsx` "groups tasks and quick chats under workspace headings" |
| .3 | `apps/web/lib/settings/agent-profile-delete-conflict.test.ts` ordering and tie-break cases |
| .4 | Go test: task with no workspace row returns empty name; helper test empty-name group sorts last; component test "falls back to the store name, then Unknown workspace" |
| .5 | `pnpm run i18n:check`; existing layout tests in `agent-profile-delete-dialog.test.tsx` stay green with grouped content |
| .6 | Existing `agent-profile-delete-dialog.test.tsx` and `profile_crud_watcher_deps_test.go` suites unchanged and green |

## E2E tests

`apps/web/e2e/tests/settings/agent-profile-delete.spec.ts`, project
`chromium`: extend "deleting profile with active task shows confirm then
conflict dialog and allows cancel" to assert the conflict dialog shows a
`profile-conflict-workspace-group` containing the seeded workspace name and
"Active Task For Profile" (AC .1).

## Work orders

- [x] [Task 01: Group delete-conflict items by workspace](task-01-group-conflict-items-by-workspace.md)

## Verification results

See [Task 01 results](task-01-group-conflict-items-by-workspace.md#results). All
work-order checks pass; unrelated local environment failures are recorded there.

## Risks

- `workspaces` table name or `tasks.workspace_id` nullability differs between
  SQLite and Postgres dialects; the query uses portable SQL and `COALESCE`.
- Other `GetActiveTaskInfoByAgentProfile` mocks construct `ActiveTaskInfo`
  positionally; additive named fields keep them compiling.
