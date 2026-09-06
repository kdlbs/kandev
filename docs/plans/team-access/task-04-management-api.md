---
id: "04-management-api"
title: "Management API, roles, and user directory"
status: todo
wave: 2
depends_on: ["03-reach-and-scope-resolution"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/org-units.md"
---

# Task 04: Management API, Roles, and User Directory

## Acceptance

- `PATCH /api/v1/workspaces/{id}` accepts `visibility`, gated on
  `workspace.manage`.
- `POST /api/v1/workspaces/visibility/bulk` sets visibility across the caller's
  own workspaces, for the one-time opt-in.
- `PATCH /api/v1/orgs/current/settings` accepts `default_workspace_visibility`,
  gated on `org.settings.manage`.
- `GET/PUT/DELETE /api/v1/workspaces/{id}/members[/{userId}]` list, add-or-
  re-role, and remove, gated on `workspace.read` / `member.manage`. `PUT` is
  idempotent and carries the workspace role.
- `POST /api/v1/workspaces/{id}/transfer-ownership` moves the accountable
  owner, demotes the previous owner to `collaborator`, and updates
  `workspaces.owner_id` in the same transaction. A target with no row is
  refused.
- Removing the last `owner` row, or the row matching `owner_id`, is refused with
  a distinct code.
- Setting `visibility = 'org'` on a workspace owned by a `guest` is refused.
- Adding a nonexistent, disabled, or already-present user returns 400 with a
  distinct code per case and writes nothing.
- `PATCH /api/v1/users/{id}` accepts the four org roles, gated on
  `org.members.manage`, with the last-owner and self-change guards.
- `GET /api/v1/users/directory` returns **only** id and display name for active
  users. A pinning test over the response field set fails if email, role, or
  status is ever added.
- Workspace DTOs carry `visibility`, `member_count`, `viewer_role`, and
  `scopes`; `GET /api/v1/auth/me` carries `org_scopes`.

## Verification

- `go test ./internal/task/... ./internal/user/... ./internal/auth/... -run 'TestVisibility|TestMember|TestTransferOwnership|TestUserDirectory|TestRoleChange'`
- `go test ./internal/user/... -run TestUserDirectoryFieldSet`

## Files Likely Touched

- `apps/backend/internal/task/handlers/workspace_handlers.go`
- `apps/backend/internal/task/service/service_members.go`
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/user/handlers/handlers.go`, `internal/user/dto/dto.go`
- `apps/backend/internal/auth/httpapi/handlers.go`

## Inputs

- Spec: API surface, Permissions, Failure modes; `roles-and-scopes.md`
  Permissions.
- Patterns: existing workspace handler shapes; the no-existence-leak rule for
  anything reachable cross-user.

## Output Contract

Report the error-code matrix, the directory field pinning test and the mutation
proving it fails, RED/GREEN commands, and set this task plus its plan checkbox
to done.
