---
status: current
system: tasks
requirements:
  - REQ-TASKS-WORKSPACE-ROOT-FILES-001
  - REQ-TASKS-WORKSPACE-ROOT-FILES-002
  - REQ-TASKS-WORKSPACE-ROOT-FILES-003
---

# Workspace Root Files System Design

## Purpose and boundaries

The tasks system owns the multi-repository task workspace layout. A bare task
root holds one checkout per attachment. agentctl already builds a root
`WorkspaceTracker` for that directory. It has `suppressGitPublication = true`,
and it owns Files events but no Git scope
(`apps/backend/internal/agentctl/server/process/manager_submodules.go`). This
design gives that root tracker one more read-only responsibility: discovering
regular files outside every checkout.

This design uses, and does not redefine:

- Per-repository status, untracked-file eligibility, and Changes groups:
  [workspace Git status](../../platform/system-design/workspace-git-status.md)
  and [multi-branch live commits](multi-branch-live-commits.md).
- The tracker graph and rescan contract:
  [Git repository scope lifecycle](../../platform/system-design/git-repository-scope-lifecycle.md).
- File reading and `workspace.file.rename` containment
  (`WorkspaceTracker.GetFileContent`, `WorkspaceTracker.RenameFile`).

No Git command runs on this path. Workspace root files never enter
`GitStatusUpdate`, so they can never become a stageable unnamed repository.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-WORKSPACE-ROOT-FILES-001` | [Discovery](#discovery), [Change notification](#change-notification), [Frontend](#frontend) |
| `REQ-TASKS-WORKSPACE-ROOT-FILES-002` | [Frontend](#frontend), [Security](#security) |
| `REQ-TASKS-WORKSPACE-ROOT-FILES-003` | [Move](#move) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| Root `WorkspaceTracker` (agentctl) | Holds the canonical checkout roots that the manager supplies. Walks the bare root, bounded. Detects membership or metadata changes on its monitor tick, and emits `file_change` notifications. |
| `process.Manager` (agentctl) | Sets checkout roots on the root tracker after each tracker-graph build, rebind, and rescan. Answers the list request only when the active root tracker is a bare multi-repository root. |
| agentctl API | `GET /api/v1/workspace/root-files`. |
| Runtime agentctl client | `ListWorkspaceRootFiles(ctx)`. |
| Backend WS handler (`internal/agent/handlers/workspace_file_handlers.go`) | Action `workspace.root_files.list` keyed by `session_id`. Uses `GetOrEnsureExecution` like `workspace.tree.get`. |
| Web store and hook | `workspaceRootFiles.byEnvironmentId[envId]` in the session-runtime slice. `useWorkspaceRootFiles(sessionId)` in `hooks/domains/session/` fetches the list, then refetches on file-change notifications. |
| Changes UI | An **Outside repositories** section rendered by `ChangesPanelBody`, shared by desktop and phone, plus a destination dialog for the move action. |

## Data and contracts

### Discovery

`ListWorkspaceRootFiles` walks `workDir` depth-first in lexical order. It uses
`os.ReadDir` and `Lstat` only.

- Skip any directory that is a configured checkout root. Also skip any
  directory that has a direct `.git` entry. That covers nested or untracked
  repositories.
- Skip directories whose name starts with `.`, and directories named
  `node_modules`.
- Skip every symbolic link, whether it points to a file or a directory. Never
  follow one.
- Skip the root ownership marker (`isRootOwnershipMarkerPath`). Skip paths
  matched by `recoveryArtifactExclusions`.
- Emit only regular files. Stop after 500 entries and set `truncated = true`
  when another eligible file exists.

The 500-entry cap and the ordering are constants in the new file
`workspace_root_files.go`.

### Wire types (`internal/agentctl/types`)

```go
type WorkspaceRootFile struct {
    Path       string    `json:"path"`        // root-relative, slash-separated
    Size       int64     `json:"size"`
    ModifiedAt time.Time `json:"modified_at"`
}

type WorkspaceRootDestination struct {
    RepositoryName string `json:"repository_name"` // matches Changes repository groups
    Path           string `json:"path"`            // root-relative checkout directory
}

type WorkspaceRootFiles struct {
    Supported    bool                       `json:"supported"`
    Files        []WorkspaceRootFile        `json:"files"`
    Truncated    bool                       `json:"truncated"`
    Destinations []WorkspaceRootDestination `json:"destinations"`
}
```

`Supported` is false when the active root tracker is not a bare
multi-repository root. In that case `Files` and `Destinations` are empty
(`AC-TASKS-WORKSPACE-ROOT-FILES-001.5`). `Destinations` comes from the
manager's repository-scope trackers whose work directory is under the root, in
the order Changes uses for repository groups. Submodule trackers are excluded.

The WS response for `workspace.root_files.list` uses the same JSON body.

### Change notification

The root tracker's monitor tick runs `observeWorkspaceRootFiles` only when
`suppressGitPublication` is set. The method mirrors
`observeWorktreeInventory`. It walks the root, then compares the result with
the last snapshot by `(path, size, modified_at)`. For each added, removed, or
modified path, it calls the existing `notifyFileChange` with `create`,
`remove`, or `write`. These notifications travel the existing
`file_change` → lifecycle → `session.workspace.file.changes` path, with an
empty `repository_name`. The first tick only seeds the snapshot. A walk error
is logged at debug level and keeps the previous snapshot.

## Control flow

```text
Agent writes <root>/dashboard.json
  -> root tracker monitor tick -> walk differs -> notifyFileChange(create)
  -> session.workspace.file.changes (repository_name "")
  -> useWorkspaceRootFiles debounced refetch (300 ms)
  -> workspace.root_files.list -> agentctl GET /workspace/root-files
  -> store workspaceRootFiles.byEnvironmentId[env] -> Outside repositories
```

The hook fetches when Changes mounts, when the session's environment changes,
and when the WS reconnects. It refetches after a file-change notification for
the session whose changes include an empty `repository_name`. A response is
applied only when its request generation is still the latest one for that
environment.

### Move

**Move to repository** opens a dialog. The dialog has a radio list of
`Destinations` with no default selection, and **Move** stays disabled until the
user selects one. On confirm, the frontend calls the existing
`renameFile(client, sessionId, path, "<destination.path>/<path>")` without
`repo`. The agentctl handler resolves both paths under the root tracker, so
the move stays inside one root (`RenameFile` rejects cross-root moves) and
`validateTargetAvailableRooted` refuses an existing target
(`AC-TASKS-WORKSPACE-ROOT-FILES-003.3`). `RenameFile` creates missing parent
directories and emits rename notifications. That removes the file from the
section. The destination's repository tracker then reports the file as
untracked through normal polling (`AC-TASKS-WORKSPACE-ROOT-FILES-003.2`). No
staging call is made.

### Frontend

- `ChangesPanelBody` renders `<ChangesRootFilesSection>` after the working-tree
  timeline when `supported && files.length > 0`. When the timeline would
  render `EmptyChangesPanel` but root files exist, the section replaces the
  empty state (`AC-TASKS-WORKSPACE-ROOT-FILES-001.1`). The section is inside
  the single `ChangesPanelBody` scroller. Because the phone sheet also mounts
  `ChangesPanelBody`, both views share it.
- Rows show the file icon and root-relative path. Activating a row calls the
  panel's existing `onEditFile(path)` with no repository. That is the same
  root-scoped file open that chat uses (`AC-TASKS-WORKSPACE-ROOT-FILES-002.1`).
- Fine pointer: a row action menu with **Open** and **Move to repository…**.
  Coarse pointer: the same items in the touch row's existing menu. Below
  640px, the shared DropdownMenu renders as a bottom sheet. Touch rows and the
  dialog's radio rows are at least 44px.
- A truncation note appears below the list when `truncated` is true.
- All copy is localized in seven locales in the `task` namespace (`src/locales/<locale>/task.json`).

## Failure and recovery

- List failure: the section keeps its last accepted list and shows no new
  error. Per-repository content is unaffected. The next notification or remount
  retries.
- Move failure (an existing target, a missing source, or a permission error):
  show a localized error toast with the backend message. Close nothing, and do
  not refresh optimistically. Notifications reflect the actual filesystem.
- A tracker-graph rebuild resets the root snapshot and checkout roots. The
  next tick seeds the snapshot again without emitting notifications, and the
  hook refetches on remount or reconnect.

## Persistence

None. The list is derived from the filesystem on every request and tick.
Restart behavior is the same as for Files.

## Security

- The WS action is keyed by `session_id`. The gateway dispatch backstop
  authorizes it, and so does `GetOrEnsureExecution`, as for
  `workspace.tree.get`.
- Discovery is confined to `workDir`. It never follows a symbolic link, and it
  only exposes paths, sizes, and modification times. File content uses the
  existing read path.
- The move reuses `RenameFile`'s rooted `os.Root` containment and its
  no-overwrite check. Destinations are validated server-side because both
  paths must resolve inside the root tracker. The client-supplied destination
  list is advisory only.

## Observability

Debug log `workspace.root_files` with `count`, `truncated`, and walk duration
when the snapshot changes. There are no metrics, because detection is
local and inexpensive. File-change notifications already appear in the
existing debug event log.

## Related decisions

- [ADR 0013: multi-branch tasks](../../../decisions/0013-multi-branch-tasks.md)
