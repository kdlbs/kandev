---
id: "02-visibility-membership-storage"
title: "Visibility and membership storage"
status: todo
wave: 1
depends_on: ["01-scope-registry"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/org-units.md"
---

# Task 02: Visibility and Membership Storage

## Acceptance

- `workspaces.visibility` (`org | private`, `NOT NULL DEFAULT 'private'`) is
  added via an idempotent `ADD COLUMN` migration.
- `workspace_members` exists with the spec's columns,
  `PRIMARY KEY (workspace_id, user_id)`, and cascade deletes from both
  `workspaces` and `users`. Replay-safe on fresh and existing databases, on
  SQLite and Postgres.
- **The migration sets every existing workspace to `private`.** A test asserts
  no existing workspace becomes org-visible — an upgrade never widens access.
- The migration writes an `owner` row for every workspace with a non-empty
  `owner_id`, and nothing for pre-auth workspaces (`owner_id = ''`).
- Workspace creation writes the owner row in the same transaction, and applies
  the org's default visibility setting.
- The org default visibility setting is persisted and defaults to `private` on
  upgrade, so an existing instance changes nothing until someone opts in.
- A consistency test asserts `workspaces.owner_id` always has a matching `owner`
  membership row, across create, transfer, and backfill.
- Workspace deletion removes membership rows both by cascade and explicitly from
  the workspace-deleted handler; the create → delete lifecycle is covered.

## Verification

- `go test ./internal/task/repository/... -run 'TestWorkspaceMembers|TestVisibilityMigration|TestOwnerBackfill'`
- `KANDEV_TEST_POSTGRES_DSN=... go test ./internal/task/repository/sqlite/...`

## Files Likely Touched

- `apps/backend/internal/task/repository/sqlite/{base_schema,base_migrations,workspace}.go`
- `apps/backend/internal/task/repository/sqlite/workspace_members.go`
- `apps/backend/internal/task/models/models.go`
- Org/system settings store for the default-visibility value

## Inputs

- Spec: Data model, Persistence guarantees (the upgrade never widens access).
- Patterns: ADR 0027 replayable migrations; the workspace-deletion side-table
  rule in `apps/backend/AGENTS.md`.

## Output Contract

Report fresh-DB and replay results on both dialects, the backfill counts
including the pre-auth zero case, the no-workspace-became-org-visible assertion,
RED/GREEN commands, and set this task plus its plan checkbox to done.
