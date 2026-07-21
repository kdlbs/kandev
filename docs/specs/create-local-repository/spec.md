---
status: shipped
created: 2026-07-21
owner: kandev
---

# Create a Local Repository During Task Creation

## Why

Users can create tasks from an existing local or remote repository, but a new project requires
leaving the task flow to initialize and register a Git repository first. Users need to create that
repository on the machine running Kandev, select it, and continue composing the same task.

## What

- The local repository selector in **Create New Task** exposes a visible **Create new repository**
  action while the task has exactly one repository row.
- The creation form asks for a repository name and an existing parent folder. It shows the resulting
  target path before confirmation. The name is one platform-native path segment, not a path.
- The parent-folder browser lists the filesystem of the machine running Kandev and shares the same
  directory browsing behavior as the **None** source mode's starting-folder picker.
- Confirmation creates `<parent>/<name>` only when that target does not already exist, initializes a
  local Git repository with unborn default branch `main`, and does not create files or commits.
- A successfully initialized repository is persisted as a local repository in the active workspace,
  added to the in-memory workspace repository list, selected in the originating row, and displays
  `main` as that row's branch. Other task fields and repository rows are unchanged.
- A commitless repository can run only with a direct local executor. Confirmation automatically
  selects a compatible `local` or `local_pc` executor profile for the first task and explains the
  change. If no direct local profile is available, confirmation is blocked before filesystem
  mutation. After the repository has a commit, later tasks may use any otherwise compatible
  executor through the normal task flow.
- Multi-repository task creation does not expose the action because those tasks require the worktree
  executor. Users can add the repository to a multi-repository task after it has its first commit.
- Initialization is an explicit local-path trust grant for the exact canonical repository path. It
  does not widen automatic discovery roots or grant access to sibling paths.
- The form remains open with the entered name and parent folder when initialization fails, surfaces
  an actionable error, and does not select a repository.
- Closing the creation form before confirmation does not create a directory or repository record.
- Desktop and mobile provide the same capability. Mobile uses a touch-native picker surface with no
  hover-only action and keeps repository name, target path, errors, and the primary action reachable.

The explicit local repository trust contract remains defined by
[Local Workspace Repositories](../workspaces/local-repositories.md) and
[ADR-2026-07-20](../../decisions/2026-07-20-explicit-local-repository-trust.md).

## Data Model

No new database entity is introduced. A successful initialization creates one existing
`repositories` record:

| Field | Contract |
| --- | --- |
| `workspace_id` | The active task-creation workspace. |
| `name` | The user-supplied repository name after trimming. |
| `source_type` | `local`. |
| `local_path` | The canonical absolute `<parent>/<name>` path. |
| `default_branch` | `main`. |

The filesystem repository and the `repositories` row are created as one product operation when the
repository form is confirmed, before the task is submitted. A successful response means both exist;
a failed response must not leave a repository row. The filesystem cleanup behavior for a partial
failure is described under Failure modes.

## API Surface

`POST /api/v1/workspaces/:id/repositories/initialize-local`

Request:

```json
{
  "name": "new-project",
  "parent_path": "/home/user/projects"
}
```

Response: `201 Created` with the existing `Repository` DTO. The response has `source_type: "local"`,
the canonical `local_path`, and `default_branch: "main"`.

The backend, not the browser, is authoritative for validation and filesystem changes:

- `400 Bad Request`: missing/invalid name, relative parent path, inaccessible parent, parent is not a
  directory, or invalid request body.
- `404 Not Found`: workspace does not exist.
- `409 Conflict`: `<parent>/<name>` already exists. Existing directories are never converted or
  overwritten by this operation.
- `500 Internal Server Error`: Git initialization or repository persistence fails.

## Permissions

This follows Kandev's trusted-local-user model. The user may initialize a repository under any
explicitly selected parent directory that the Kandev process can write. The grant is scoped to the
created canonical target path and the active workspace.

## Failure Modes

| Condition | Observable behavior |
| --- | --- |
| Name is empty, `.`/`..`, or contains a host path separator | Confirmation is disabled client-side and the backend rejects a forged request. |
| Parent path cannot be read or written | The form stays open, shows an error, and no repository is selected or persisted. |
| Target already exists, including an empty directory | The request returns conflict and does not modify the target. |
| `git` is unavailable or initialization fails | The request fails, no repository row is persisted, and Kandev removes only the target directory created by this request when cleanup is safe. A cleanup failure is logged and the error remains visible. |
| Repository persistence fails after Git initialization | The request fails and performs the same best-effort cleanup of the request-owned target; no repository row is returned. |
| Frontend state refresh fails after a successful response | The returned repository is still selected directly and merged into the active workspace cache without waiting for a second list request. |
| No direct local executor profile is available | The form explains that an empty repository requires local execution and does not call the initialization endpoint. |

## Persistence Guarantees

The initialized Git repository is ordinary user-owned filesystem content and is never tied to task
deletion. Its existing `repositories` record survives backend restart. Canceling, archiving, or
deleting the task does not delete the repository or its workspace registration.

## Mobile Design Contract

- **Entry and outcome:** the repository chip opens the same local repository picker as desktop; a
  visible, at-least-44px **Create new repository** row opens the creation flow and returns the new
  selection to the originating chip.
- **Nearest exemplar:** `MobilePickerSheet` supplies the inset bottom-drawer geometry, fixed header,
  internal scrolling, and predictable dismissal used by task-workbench pickers.
- **Hierarchy and primary action:** repository name and current target path stay above the directory
  list; **Create repository** is the single primary action in the safe-area-aware footer.
- **Presentation rationale:** repository creation is a short temporary choice inside task creation,
  so mobile uses an inset bottom drawer rather than a new route or a compressed desktop dialog.
- **Geometry:** the drawer uses one `min-h-0` vertical scroll owner, dynamic viewport sizing, bottom
  safe-area clearance, no document horizontal overflow, and 44px directory/action rows. Keyboard
  focus remains visible and dismissal returns focus to the repository selector.
- **Shared logic:** validation, target-path derivation, directory listing, API mutation, workspace
  cache update, row selection, and compatible local-executor selection are shared. Only
  Dialog-versus-Drawer composition is responsive.

## Scenarios

- **GIVEN** the task dialog has a local repository row, **WHEN** the user chooses **Create new
  repository**, selects `/work/projects`, enters `alpha`, and confirms, **THEN**
  `/work/projects/alpha` contains a commitless Git repository whose unborn branch is `main`, the
  workspace has a matching local repository record, the originating task row selects `alpha` /
  `main`, and the first task uses a direct local executor.
- **GIVEN** a worktree or container executor profile is selected and a direct local profile is
  available, **WHEN** repository initialization succeeds, **THEN** the form selects the compatible
  direct local profile and tells the user why the executor changed.
- **GIVEN** no direct local executor profile is available, **WHEN** the user opens the creation form,
  **THEN** the primary action explains the requirement and no directory or repository is created.
- **GIVEN** a task with multiple repository rows, **WHEN** the user opens any repository picker,
  **THEN** **Create new repository** is not offered because an unborn repository cannot use the
  required worktree executor.
- **GIVEN** `/work/projects/alpha` already exists, **WHEN** the user tries to create `alpha` under
  `/work/projects`, **THEN** the UI reports the conflict and Kandev does not modify or register the
  existing path.
- **GIVEN** an invalid name or unwritable parent, **WHEN** the user confirms, **THEN** the form stays
  open with its inputs intact and no repository is selected.
- **GIVEN** the creation form is open, **WHEN** the user dismisses it without confirming, **THEN** no
  filesystem or database change occurs and their task draft remains intact.
- **GIVEN** a repository was initialized and selected, **WHEN** the user later cancels or deletes the
  task, **THEN** the repository directory and workspace repository record remain.
- **GIVEN** a Pixel 5 viewport, **WHEN** the user completes the same flow, **THEN** all actions are
  reachable by touch, the directory list scrolls inside the drawer, the footer clears the safe area,
  and the page has no horizontal overflow.

## Out of Scope

- Creating or publishing a remote Git hosting repository.
- Cloning, importing, or converting an existing directory; existing paths continue through the
  existing local repository selection and settings flows.
- README, license, `.gitignore`, language template, remote, visibility, or hosting-provider setup.
- Choosing the initial branch name or creating an initial commit.
- Worktree, container, or remote execution before the repository has its first commit.
- Creating an unborn repository as part of a multi-repository task.
- Adding the creation action to Quick Chat, repository settings, project repository pickers, or MCP
  tools in this iteration.
- Automatically deleting a successfully created repository when its originating task is canceled or
  deleted.

## Implementation Plan

See [Create Local Repository plan](../../plans/create-local-repository/plan.md).
