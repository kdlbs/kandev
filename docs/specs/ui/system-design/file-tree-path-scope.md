---
status: current
system: ui
requirements:
  - REQ-UI-FILE-TREE-PATH-SCOPE-001
---

# File Tree Path Scope System Design

## Purpose and boundaries

The UI owns the path identity passed from agent tool cards into file editors and the relative
directory identities retained by the task Files tree. The agentctl workspace file service owns the
filesystem boundary and provides defense in depth for direct tree requests.

This design distinguishes two existing contracts. File content reads may use an external absolute
path under [ADR 0016](../../../decisions/0016-observed-external-file-reads.md). File-tree reads are
directory enumeration inside the task workspace and accept only workspace-relative paths. An
absolute editor identity therefore never implies an absolute tree identity.

## Requirement mapping

| Acceptance criterion               | Design sections                                                                              |
| ---------------------------------- | -------------------------------------------------------------------------------------------- |
| `AC-UI-FILE-TREE-PATH-SCOPE-001.1` | [Path identities](#path-identities), [Tool-file open flow](#tool-file-open-flow)             |
| `AC-UI-FILE-TREE-PATH-SCOPE-001.2` | [Tree-state admission](#tree-state-admission), [Failure and recovery](#failure-and-recovery) |
| `AC-UI-FILE-TREE-PATH-SCOPE-001.3` | [Tree-state admission](#tree-state-admission), [Refresh flow](#refresh-flow)                 |
| `AC-UI-FILE-TREE-PATH-SCOPE-001.4` | [Persistence](#persistence)                                                                  |
| `AC-UI-FILE-TREE-PATH-SCOPE-001.5` | [Backend validation](#backend-validation), [Observability](#observability)                   |
| `AC-UI-FILE-TREE-PATH-SCOPE-001.6` | [Responsive behavior](#responsive-behavior), [Verification](#verification)                   |

## Components and responsibilities

- A shared pure frontend path helper classifies POSIX and Windows absolute paths, performs
  segment-boundary containment against the active task workspace root, converts contained aliases
  to slash-separated relative paths, and validates paths admitted to tree state.
- `useOpenFileAtLine` in `apps/web/hooks/use-file-editors.ts` normalizes tool-originated paths before
  it keys pending cursor state, calls the editor opener, or scrolls an existing editor. Tool read and
  edit cards therefore use one path identity for file content, tabs, cursor state, and the Files
  panel.
- `useFileTreeReveal` admits only valid workspace-relative active paths before calculating ancestors
  or loading children.
- `restoredExpandedPaths` validates stored entries before synthesizing their ancestor chains.
- `applyFileChanges` filters expanded paths through the same tree-path predicate before a generic
  refresh fans out `workspace.tree.get` requests.
- `WorkspaceFileHandlers.wsGetFileTree` rejects invalid client paths before execution lookup and
  returns a WebSocket validation error without logging an operational failure.
- `WorkspaceTracker.GetFileTree` repeats the absolute and traversal check before joining the request
  to `workDir`, so direct agentctl HTTP callers cannot reach a filesystem lookup with an invalid
  tree path.

## Path identities

The frontend path helper returns one of these outcomes:

| Input                       | Workspace root | Open identity           | Tree eligible |
| --------------------------- | -------------- | ----------------------- | ------------- |
| `src/app.ts`                | `/workspace`   | `src/app.ts`            | yes           |
| `/workspace/src/app.ts`     | `/workspace`   | `src/app.ts`            | yes           |
| `/workspace-old/src/app.ts` | `/workspace`   | unchanged absolute path | no            |
| `/opt/reference.md`         | `/workspace`   | unchanged absolute path | no            |
| `../sibling/app.ts`         | `/workspace`   | unchanged invalid path  | no            |

Containment uses a complete path segment, normalizes `\` to `/`, and compares Windows drive paths
case-insensitively. A sibling whose prefix happens to share the workspace root string is not
contained. The empty string is valid only as the tree root. A relative tree path rejects empty
segments introduced by a leading slash, `.` and `..` segments, file URI syntax, and Windows drive
roots.

The helper does not rewrite a legitimate external absolute file into a task-relative lookalike.
That path continues to the read-only content endpoint unchanged and remains ineligible for the tree.

## Tool-file open flow

1. The normalized ACP read or edit message supplies its recorded file path and optional source line.
2. The tool renderer passes the path through `useOpenFileAtLine` with the active session workspace
   root.
3. If the path is contained by that root, the hook derives the workspace-relative identity before
   writing pending cursor state and calling `onOpenFile`.
4. The existing file editor requests content, creates or focuses the tab, and scrolls using the same
   normalized identity.
5. `FilesPanel` observes the active relative editor identity and may reveal its valid ancestor chain.

An external absolute path follows the same file-content route without step 3's rewrite. The editor
can display it, while the reveal boundary ignores it.

## Tree-state admission

Tree state uses one predicate at each source boundary rather than assuming every caller has already
normalized its input:

- Active-file reveal returns before changing expansion state when the path is invalid.
- Restored session storage drops invalid entries before deriving parents.
- Generic refresh ignores invalid entries already present in memory and always retains the valid
  root refresh.
- Specific file-change events continue to refresh the nearest valid expanded ancestor. Invalid
  change paths are ignored rather than converted into a host path.

The predicate applies to state and request identities, not ordinary filenames shown in labels.
Hidden paths such as `.codex/agents` remain valid.

## Refresh flow

A generic file refresh starts with the root request `""`. For each valid expanded directory in the
affected repository scope, `applyFileChanges` schedules one relative child request. Invalid entries
do not enter the request set. Responses continue to merge with loaded descendants through the
existing `mergeTreeNodes` behavior.

This filter is required even after tool-path normalization because an existing browser tab may hold
state written by an older build or by another editor entry point.

## Backend validation

The WebSocket handler validates the tree path before it acquires an agentctl client. Invalid paths
return `ErrorCodeValidation`; they are client contract failures and do not emit the
`failed to get file tree` ERROR log.

The agentctl process validates again because its HTTP endpoint is independently callable. It rejects
an absolute path or a cleaned parent escape before `filepath.Join`, `os.Stat`, or filesystem-failure
telemetry. Valid relative paths retain the existing containment check, missing-path behavior, depth
handling, symlink behavior, and tree construction.

## Failure and recovery

- An invalid active editor path has no tree side effect. The editor's own content result remains the
  only user-visible success or failure.
- Invalid stored entries are pruned locally; valid siblings restore normally, and the cleaned set is
  written back through the existing session-storage effect after load.
- A malformed direct WebSocket request receives a validation response. A direct agentctl HTTP
  request receives the existing 400 response with a bounded validation message.
- A valid relative tree path that disappears can still return the existing missing-path error. That
  operational behavior is outside this repair.

## Persistence

No storage schema changes. Files-panel expansion remains scoped by task environment in
`sessionStorage`. Restore sanitization is a forward cleanup for entries produced by older clients;
the next normal persistence pass replaces the contaminated array with the accepted relative set.

Open external absolute file tabs retain their existing persistence and read-only reopen behavior.
They are not copied into expansion storage.

## Security

Frontend validation limits accidental and stale requests but is not an authorization boundary. The
agentctl process remains authoritative and rejects absolute or escaping tree enumeration. Symlink
containment continues through existing workspace tracker behavior.

ADR 0016 remains limited to current-file content reads. This design does not expand absolute access
to directory enumeration, mutation, search, Git-ref reads, or any other workspace operation.

## Responsive behavior

No component anatomy, styling, touch target, copy, focus path, or scroll owner changes. Desktop tool
links continue to open or focus the Dockview editor. Phone tool links continue to switch to the
native `MobileFileViewerPanel`. Both receive the same normalized workspace-relative identity and
line target.

The phone regression uses the existing chat-read-to-CodeMirror flow as its nearest exemplar. A
separate mobile layout implementation is unnecessary because normalization happens before the
desktop and phone openers diverge.

## Verification

- Pure helper tests cover POSIX paths, Windows drive paths, segment-boundary prefix collisions,
  separator normalization, external absolutes, file URIs, and traversal.
- `useOpenFileAtLine` tests first reproduce the defect by expecting `/workspace/src/app.ts` to open
  and scroll as `src/app.ts`; this assertion fails before the production change. They also prove an
  external absolute path remains unchanged.
- Reveal, restore, and refresh tests seed the reported absolute expansion chain and assert no load or
  request receives `/home`, `/home/user`, or descendants while valid relative siblings remain.
- Backend process tests assert invalid paths fail before lookup. Handler tests assert an absolute
  path returns `ErrorCodeValidation`, does not consult lifecycle, and produces no ERROR log.
- Desktop and phone Playwright scenarios seed an agent read card with the active workspace's
  absolute file path, activate it, and assert the existing editor/viewer opens the relative path at
  the requested line. The desktop scenario also observes `workspace.tree.get` traffic and fails if
  any request carries an absolute path.

## Observability

No new metric or routine log is needed. Invalid direct tree requests are represented by their
validation response. Genuine dependency or filesystem failures keep their current logs. The
absence of absolute tree requests is enforced by frontend and backend regression tests.

## Related decisions

- [ADR 0016: Read-Only Absolute File Paths](../../../decisions/0016-observed-external-file-reads.md)
