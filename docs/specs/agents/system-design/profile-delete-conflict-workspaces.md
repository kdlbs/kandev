---
status: current
system: agents
requirements:
  - REQ-AGENTS-PROFILE-DELETE-WORKSPACES-001
---

# Profile Delete Conflict Workspaces System Design

## Purpose and boundaries

The agent settings controller owns the profile delete-conflict inventory
(`ErrProfileInUseDetail`) and the HTTP 409 body built from it. The task
repository supplies the affected-task rows through the existing
`GetActiveTaskInfoByAgentProfile` query. The web conflict dialog
(`AgentProfileDeleteConflictDialog`) renders that inventory. This design adds
workspace identity to each affected-task row and groups the dialog's Tasks and
Quick Chats sections by workspace. Which sessions block deletion, force
semantics, and the other conflict sections are unchanged.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-PROFILE-DELETE-WORKSPACES-001` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `agentdto.ActiveTaskInfo` (`apps/backend/internal/agent/dto/dto.go`) gains
  `WorkspaceID` and `WorkspaceName`.
- `Repository.GetActiveTaskInfoByAgentProfile`
  (`apps/backend/internal/task/repository/sqlite/session.go`) selects
  `t.workspace_id` and the workspace name through
  `LEFT JOIN workspaces w ON w.id = t.workspace_id`, with
  `COALESCE(w.name, '')`. Its `WHERE` clause and `DISTINCT` semantics are
  unchanged. The left join keeps a task whose workspace row is missing.
- `ErrProfileInUseDetail.ActiveSessions` and the 409 handler in
  `apps/backend/internal/agent/settings/handlers/profile_handlers.go` pass the
  rows through unchanged.
- `ActiveSessionInfo` (`apps/web/lib/types/agent-profile-errors.ts`) gains
  optional `workspace_id` and `workspace_name`, optional so a response from an
  older backend still renders.
- A pure helper `groupSessionsByWorkspace(sessions, resolveName)` in
  `apps/web/lib/settings/agent-profile-delete-conflict.ts` returns ordered
  groups `{ workspaceId, workspaceName, sessions }[]` per
  `AC-AGENTS-PROFILE-DELETE-WORKSPACES-001.3`. It keys groups by
  `workspace_id` (empty string for a missing ID). A group whose rows carry no
  `workspace_name` resolves it through the caller-supplied `resolveName`
  lookup before ordering, so groups sort by the name the dialog shows;
  response-provided names stay authoritative.
- `SessionConflictSection` in
  `apps/web/components/settings/agent-profile-delete-dialog.tsx` renders the
  section title, then one workspace group per helper result: a heading and a
  list of item titles. The heading is an `h3` so heading navigation reaches
  each workspace group. The heading label is `workspace_name`, else the name of
  the matching entry in `useAppStore((s) => s.workspaces.items)`, else
  `t("agents:conflictUnknownWorkspace")`. Each group carries
  `data-testid="profile-conflict-workspace-group"`.

## Data and contracts

HTTP 409 body for `DELETE /api/v1/agent-profiles/:id`, `active_sessions`
entries:

```json
{
  "task_id": "t-1",
  "task_title": "Fix login",
  "is_ephemeral": false,
  "workspace_id": "ws-1",
  "workspace_name": "Backend"
}
```

`workspace_id` and `workspace_name` are always present from this backend;
`workspace_name` is `""` when the workspace row is missing. Clients treat both
as optional. No WebSocket or persistence contract changes.

New locale key `agents:conflictUnknownWorkspace` ("Unknown workspace") in all
shipped locales; Traditional Chinese through `pnpm run i18n:zh-hant`.

## Control flow

1. `prepareProfileDeletion` calls `GetActiveTaskInfoByAgentProfile`, which now
   returns workspace identity per row.
2. The handler serializes the rows into `active_sessions`.
3. `deleteAgentProfileError` passes them through to the dialog.
4. The dialog splits rows into tasks and quick chats (`is_ephemeral`), groups
   each list with `groupSessionsByWorkspace`, and renders the groups inside
   the existing scrollable `agent-profile-delete-conflict-body`.

## Failure and recovery

- Missing workspace row or older backend without the fields: the item is
  grouped under the empty or matching ID and labelled from the store or
  "Unknown workspace" (`AC-AGENTS-PROFILE-DELETE-WORKSPACES-001.4`).
- Query errors keep their existing behavior: the delete request fails.

## Persistence

None. The query reads existing `tasks.workspace_id` and `workspaces.name`.

## Security

Workspace names are shown only to callers already allowed to delete the
profile, who already see the affected task titles. No new data is exposed
beyond names of workspaces containing those tasks.

## Observability

No new logs or metrics.

## Related decisions

None. This extends an existing response shape additively.
