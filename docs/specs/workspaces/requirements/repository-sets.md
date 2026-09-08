---
status: active
system: workspaces
created: 2026-08-17
owners:
  - kandev
---
# Repository Sets Requirements

## Overview

A workspace can hold dozens of repositories, and real work rarely touches one of them. A feature that runs from a web client through an API gateway into two backend services means the same four repositories are picked, one chip at a time, every time a task is created. The selection is knowledge the user already has and re-enters by hand on every task, and a repository forgotten on the third of four chips is only discovered once the agent cannot find the code.

## Requirements

### REQ-WORKSPACES-REPOSITORY-SETS-001: Repository Sets

**Intent:** A workspace can hold dozens of repositories, and real work rarely touches one of them. A feature that runs from a web client through an API gateway into two backend services means the same four repositories are picked, one chip at a time, every time a task is created. The selection is knowledge the user already has and re-enters by hand on every task, and a repository forgotten on the third of four chips is only discovered once the agent cannot find the code.

#### Acceptance criteria

- **AC-WORKSPACES-REPOSITORY-SETS-001.1:** A workspace owns any number of repository sets. A set has a name, an optional description, and an ordered list of repositories drawn from that same workspace.
- **AC-WORKSPACES-REPOSITORY-SETS-001.2:** Sets are managed from **Settings > Workspaces > _workspace_ > Repositories**, in a **Repository sets** section below the repository list. The section lists each set with its name, description, and member repositories, and offers create, rename, edit-members, reorder, and delete.
- **AC-WORKSPACES-REPOSITORY-SETS-001.3:** Sets can also be created from the task-creation dialog: **Save as set** captures the workspace repositories currently selected in the form, asks for a name, and creates the set without discarding the in-progress task.
- **AC-WORKSPACES-REPOSITORY-SETS-001.4:** A set name is trimmed, is 1 to 100 characters, and is unique within its workspace, compared case-insensitively. A second set named `Full-stack` in a workspace that already has `full-stack` is rejected and the existing set is named in the error.
- **AC-WORKSPACES-REPOSITORY-SETS-001.5:** A set holds each repository at most once. Ordering is user-controlled and is the order in which the set fills the picker.
- **AC-WORKSPACES-REPOSITORY-SETS-001.6:** Creating or updating a set requires at least one repository. A set that becomes empty because its repositories were deleted is kept, not removed.
- **AC-WORKSPACES-REPOSITORY-SETS-001.7:** Sets are workspace-scoped, not per-user: every user who can see the workspace sees and can apply its sets.
- **AC-WORKSPACES-REPOSITORY-SETS-001.8:** The repository row of the task-creation dialog gains a **Sets** control next to **add repository**, listing the workspace's sets by name with their repository count.

## Migrated source detail

## Why

A workspace can hold dozens of repositories, and real work rarely touches one of them. A feature
that runs from a web client through an API gateway into two backend services means the same four
repositories are picked, one chip at a time, every time a task is created. The selection is
knowledge the user already has and re-enters by hand on every task, and a repository forgotten on
the third of four chips is only discovered once the agent cannot find the code.

A **repository set** is a named, reusable group of workspace repositories. Choosing a set fills the
task-creation repository picker with every repository in it, in one action. Branches remain a
per-task decision and are never stored in a set.

## What

### Defining a set

- A workspace owns any number of repository sets. A set has a name, an optional description, and an
  ordered list of repositories drawn from that same workspace.
- Sets are managed from **Settings > Workspaces > _workspace_ > Repositories**, in a **Repository
  sets** section below the repository list. The section lists each set with its name, description,
  and member repositories, and offers create, rename, edit-members, reorder, and delete.
- Sets can also be created from the task-creation dialog: **Save as set** captures the workspace
  repositories currently selected in the form, asks for a name, and creates the set without
  discarding the in-progress task.
- A set name is trimmed, is 1 to 100 characters, and is unique within its workspace, compared
  case-insensitively. A second set named `Full-stack` in a workspace that already has `full-stack`
  is rejected and the existing set is named in the error.
- A set holds each repository at most once. Ordering is user-controlled and is the order in which
  the set fills the picker.
- Creating or updating a set requires at least one repository. A set that becomes empty because its
  repositories were deleted is kept, not removed.
- Sets are workspace-scoped, not per-user: every user who can see the workspace sees and can apply
  its sets.

### Applying a set

- The repository row of the task-creation dialog gains a **Sets** control next to **add
  repository**, listing the workspace's sets by name with their repository count.
- Choosing a set adds one repository row per member, in set order, leaving each row's branch empty
  so the dialog's existing per-row branch defaulting fills it. The user then reviews and adjusts
  branches exactly as when adding rows by hand.
- A single empty placeholder row is consumed by the first applied repository rather than left
  behind. Rows the user already configured are never discarded or reordered by applying a set.
- Applying a set is additive and idempotent: a repository already present in the form is skipped,
  so applying the same set twice changes nothing and applying two overlapping sets yields the union.
- A member repository that no longer exists in the workspace, or was deleted, is skipped. When any
  member is skipped the dialog reports how many were skipped and why, as visible text.
- The same control appears in the **new subtask** form, which uses the same repository picker. It
  does not appear in Quick Chat.
- The control is absent when the workspace has no sets **and** the current selection cannot be saved
  as one, so a control that could only report "you have no sets" never appears. When the form does
  hold a workspace repository, the control stays available with **Save as set** and no set entries:
  otherwise the first set could never be defined from the flow that just chose the repositories.
- The control is absent in **Remote URL** and **No repository** source modes, where workspace
  repositories are not what is being selected.
- The control is never disabled by executor capability. **Add repository** beside it is not
  executor-gated either: the repository selection is what constrains which executor profiles stay
  selectable, and the executor picker already marks an incompatible profile unavailable with its
  reason. Applying a set on a single-repository executor therefore behaves exactly like adding the
  same rows by hand.
- Applying a set changes only the form. No task, repository, or set is modified, and nothing is
  persisted until the task is created.

### Live updates

- A set created, changed, or deleted in one client appears in every other client viewing that
  workspace without a reload.

## Data model

Two new tables, both workspace-owned.

`repository_sets`

| Field                      | Contract                                                          |
| -------------------------- | ----------------------------------------------------------------- |
| `id`                       | Stable set identity.                                              |
| `workspace_id`             | Owning workspace; cascade-deleted with the workspace.             |
| `name`                     | Trimmed, 1 to 100 characters, unique per workspace.               |
| `description`              | Optional free text, defaults to empty.                            |
| `created_at`, `updated_at` | Audit timestamps.                                                 |

`(workspace_id, name)` is unique in the database, and a second unique index on
`(workspace_id, LOWER(name))` makes that backstop case-insensitive too. The service rejects a
case-insensitive collision with a message naming the existing set; the index is what stops two
concurrent creates of `Full-stack` and `full-stack` from both landing.

`repository_set_items`

| Field                      | Contract                                                            |
| -------------------------- | ------------------------------------------------------------------- |
| `id`                       | Stable membership identity.                                         |
| `repository_set_id`        | Owning set; cascade-deleted with the set.                           |
| `repository_id`            | A repository in the same workspace as the set.                      |
| `position`                 | Zero-based order within the set; contiguous after every write.      |
| `created_at`, `updated_at` | Audit timestamps.                                                   |

`(repository_set_id, repository_id)` is unique: a set cannot list the same repository twice.

Sets deliberately store **no branch**. Branch choice belongs to a task, is already modelled on
`task_repositories.base_branch` / `checkout_branch`, and is what the user still decides per task.

Repositories are soft-deleted (`repositories.deleted_at`), so a foreign-key cascade does not remove
memberships. Deleting a repository removes its `repository_set_items` rows in the same transaction
that soft-deletes it, and every read of a set also excludes members whose repository is soft-deleted
or has moved out of the workspace. Either mechanism alone is sufficient; both are present so a set
can never surface a repository the user cannot select.

## API surface

Collection routes are workspace-scoped, item routes are flat, matching the existing repository
routes.

```
GET    /api/v1/workspaces/:id/repository-sets
POST   /api/v1/workspaces/:id/repository-sets
GET    /api/v1/repository-sets/:id
PATCH  /api/v1/repository-sets/:id
DELETE /api/v1/repository-sets/:id
```

Create request:

```json
{
  "name": "Full-stack",
  "description": "web + gateway + services",
  "repository_ids": ["repo-web", "repo-gateway", "repo-orders"]
}
```

`repository_ids` is ordered and defines `position`. Update accepts `name`, `description`, and
`repository_ids` independently; a supplied `repository_ids` replaces the whole membership list,
which is also how reordering is expressed.

Response:

```json
{
  "id": "set-1",
  "workspace_id": "ws-1",
  "name": "Full-stack",
  "description": "web + gateway + services",
  "repositories": [
    { "repository_id": "repo-web", "position": 0 },
    { "repository_id": "repo-gateway", "position": 1 },
    { "repository_id": "repo-orders", "position": 2 }
  ],
  "created_at": "2026-08-17T09:00:00Z",
  "updated_at": "2026-08-17T09:00:00Z"
}
```

The same five operations are available over the WebSocket dispatcher as
`repository_set.list|create|get|update|delete`, mirroring `repository.*`, so clients on the socket
do not need a second transport.

Set membership is also exposed on the boot payload as a workspace-keyed `repositorySets`
collection, so the task-creation dialog can offer sets on first paint without a fetch.

## Permissions

- Every route resolves the workspace and applies the same workspace-access authorization already
  required for repository routes. A user without access to the workspace receives the same response
  as for a workspace that does not exist.
- Item routes (`/repository-sets/:id`) resolve the set's workspace first and authorize against it;
  a set id from another workspace is not readable, writable, or deletable.
- `repository_ids` are validated against the set's workspace. A repository id from another
  workspace is rejected and does not reveal whether that id exists.

## Failure modes

| Situation                                                    | Result                                                          |
| ------------------------------------------------------------ | --------------------------------------------------------------- |
| Malformed body, blank name, name over 100 characters          | `400`, no write                                                 |
| Empty or absent `repository_ids` on create, empty on update   | `400`, no write                                                 |
| Duplicate id inside `repository_ids`                          | `400`, no write                                                 |
| Unknown workspace, unknown set, or no workspace access        | `404`, no write                                                 |
| Name already used in the workspace, case-insensitively        | `409` naming the existing set, no write                          |
| A `repository_id` is unknown, deleted, or in another workspace | `422` listing the offending ids, no write                       |
| Repository listing fails while building the boot payload      | Boot succeeds with an empty set list; the dialog fetches instead |
| A set's members were all deleted                              | The set lists as empty and cannot be applied; it is not removed  |
| Applying a set on an executor that forbids multi-repository    | The set applies; the executor picker marks that profile unavailable |

Writes are atomic: name, description, and membership of one request either all land or none do,
because the store applies both halves in a single transaction. A set deleted between the read and
the write is reported as not found rather than resurrected.

## Persistence guarantees

- A created or updated set is durable before the response returns, and its membership positions are
  contiguous from zero in the order requested.
- Deleting a workspace deletes its sets and memberships. Deleting a set deletes its memberships and
  leaves every repository untouched. Deleting a repository removes it from every set, leaves the
  sets themselves intact, and publishes each affected set's new shape so open clients stop offering
  the deleted member.
- Applying a set writes nothing. The repositories that reach `task_repositories` are whatever the
  form holds at submit, so a set applied and then edited persists the edited selection.
- Sets survive backup and restore with the rest of the workspace schema, and the schema
  initialization is replay-safe: initializing twice against the same database is a no-op.

## Out of scope

- Branches, agent profiles, executor profiles, or workflows stored in a set. A set is repositories.
- Remote-URL and folder sources. Sets reference workspace repository entities.
- Quick Chat, and adding sources to an already-created task.
- Cross-workspace and per-user sets.
- Automatic set suggestions derived from task history.
