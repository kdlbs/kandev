---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-FILE-ENTRY-MUTATIONS-001
---

# File Entry Mutations System Design

## Ownership and current failure

Workspaces owns the mutation contract. The identification pair supplies visible
link metadata without changing access; the saved-content pair owns target
editing. Neither currently defines Delete/Rename leaf identity, so this pair
adds that missing contract. UI path scope and executor durable-source recovery
remain adjacent authorities, not new owners of this correction.

Before this correction, `DeleteFile` and `RenameFile` in
`server/process/workspace_files.go` called
`resolveMutationPath`, which calls `resolveSafePath`. Full `EvalSymlinks`
canonicalization changes a selected link into its target before the retained
`os.Root` receives the operation. `GetFileTree` exposes the selected alias with
`IsSymlink=true`, making this a mismatch between advertised and mutated entry.
Delete also used following `Stat` for classification; destination rename
validation used following `Stat`, which cannot classify a dangling
destination as occupied.

The read-only qualified ROOT proof and exact base are recorded in the
[plan](../../../plans/preserve-leaf-link-mutations/plan.md). No replay or
production/test modification was required to establish the causal failure.

## Caller and transport path

`files-panel.tsx` passes `useFileOperations` callbacks to `FileBrowser`.
`file-context-menu.tsx` sends the selected `node.path` for Delete and Rename;
`file-browser-move.ts:executeMoveFiles` sends computed old/new tree paths to
the same rename callback. Optimistic settlement already uses success/failure.
Phone touch actions share these callbacks and the same tree identity.

`lib/ws/workspace-files.ts` sends `workspace.file.delete` or
`workspace.file.rename`, with session and optional repository scope.
`WorkspaceFileHandlers.wsDeleteFile` / `wsRenameFile` forward through
`internal/agent/runtime/agentctl.Client.DeleteFile` / `RenameFile`.
Registered agentctl routes are `DELETE /api/v1/workspace/file` and
`POST /api/v1/workspace/file/rename`. Their handlers call `Manager.JoinRepoPath`
then mutate `GetWorkspaceTracker()`, the root tracker, rather than a selected
repository tracker. Requests without `repo` use aggregate tree paths directly.
No forwarding, response type, repository-selection, or frontend change is needed.

## Narrow entry resolution

Keep `resolveSafePath` and `resolveMutationPath` unchanged for read, edit,
create, upload, and other target operations. Add a narrowly scoped entry
mutation helper in `workspace_files.go` or a focused companion file.

1. Normalize the request using native `filepath` semantics and the canonical
   workspace prefix. Preserve existing admission of relative and contained
   absolute mutation paths. Handle the root explicitly before splitting a leaf.
2. Resolve the parent with existing canonical workspace/registered-source
   admission, including the existing missing-ancestor handling for rename
   destinations. Retain an `os.Root` opened at the selected authority root;
   compose its validated relative parent with the original final basename.
   Do not open the resolved parent as an independent authority root and do not
   rebuild a plain unrooted mutation path.
3. For sources, retain full-target admission and existence validation. Reject
   canonical workspace/source roots and aliases to them before a no-op. A
   resolved source target must remain in the same authority root as its entry
   parent. Preserve dangling/loop rejection rather than trusting the missing
   path fallback as authority. This keeps direct workspace leaf links into a
   registered external root rejected while allowing descendants routed through
   that registered link as a parent.
4. The operation path is the resolved parent plus leaf, never the resolved leaf
   target. Its `safe` notification path uses that entry location. Retain the
   authority-root handle through validation, barrier, and mutation, closing it
   on every return path. Any additional target-validation handle must also close.

Implementation can reuse `rootedMutationPath` and small local helpers. Do not
add a resolver policy enum to every call, a new global map, or a filesystem
abstraction. Root admission and source validation must occur before effects;
root aliases cannot become deletable merely because the preserved entry is
lexically inside the workspace.

## Operation semantics

Delete uses rooted `Lstat` to classify the leaf. Remove a symlink with rooted
`Remove`; remove an actual directory with rooted `RemoveAll`. Ordinary files
continue through `Remove`. Preserve missing-file failure classification.

Rename resolves both entry parents, rejects root operands and differing
authority roots, and compares entry-relative paths for the existing no-op.
Source eligibility still includes full-target validation with rooted `Stat` on
the canonical target. After the mutation barrier, check the preserved source
entry with rooted `Lstat`, so an eligible absolute contained link value is not
followed again by the native root operation. Validate destination
availability with rooted `Lstat`; only an absent final entry is available.
Missing ancestors remain eligible for rooted `MkdirAll`. A dangling or looping
destination link is occupied without dereferencing it. Never compare source
and destination canonical targets to infer a no-op: distinct links to the same
target are distinct entries. Use retained-root `Rename` to move the entry.

Stored absolute and relative link values are not rewritten during move. In a
new parent directory a relative link may become dangling; target bytes remain
untouched. No promise of
atomic no-clobber against concurrent within-root destination creation is added.

## Containment and race boundary

Keep `SetAllowedSourceRoots`, `allowedSourcePath`, and canonical root selection.
Registered roots grant descendant routing, not removal or relocation of their
root alias. The existing `workspaceMutationBarrier` stays between resolution
and rooted operation. A plain parent directory swapped to an external symlink
after admission must fail under retained `os.Root`, including source and
destination rename parents. A registered source is anchored by its canonical
root handle, not an unchecked alias reconstructed after validation.

Resolving a permitted parent link to its contained canonical parent preserves
the existing parent-routing behavior. This correction does not add a broad
TOCTOU policy, preclude every concurrent within-root replacement, or change
authorization when the source-root allowlist is updated mid-operation.

[ADR 0016](../../../decisions/0016-observed-external-file-reads.md) remains
read-only. The [workspace storage symlink decision](../../../decisions/2026-07-19-workspace-symlink-entries.md)
owns cleanup, not this interactive operation. No new ADR is required: this is
a local restoration of native entry semantics within existing rooted authority,
and this pair records the bounded choices sufficiently.

## Responses and notifications

Keep HTTP 200 successful responses and HTTP 400 failures, selected request
`path` or `old_path`/`new_path`, `success`, and `error`. WS clients keep existing
error conversion. No new error wording is required for existing categories.
Reuse `notifyFileChange`, `notifyRename`, and `mutationNotificationPath` after
successful effects only. For a direct leaf link, notify the alias instead of
its target. Retain canonical parent routing for linked parents; registered
source notification paths retain existing absolute-path behavior. Root-tracker
HTTP events retain `RepositoryName == ""` even for explicit `repo` requests.
Do not introduce per-repository tracker routing to make a test expectation pass.

## Verification and native qualification

Permanent tests independently recreate private real files after implementation
authorization. Use actual `WorkspaceTracker.GetFileTree`, `DeleteFile`, and
`RenameFile`, plus the registered `Server.Router()` with a real manager.
Subscribe before mutation and inspect immediate events without starting a
watcher. Use distinct target, neighbor, root, alpha, and beta sentinel bytes.
Assert `Lstat` type, exact `Readlink` value, source absence, target entry/type
and byte preservation, and unselected identities. Red must reach the real
filesystem assertions, not fail from imports, predicates, or missing APIs.

Cover file and directory link delete/rename/move; destinations containing plain
entries, readable/dangling/loop links; distinct links to one target; root aliases;
source dangling/loop rejection; nested/new destination parents; ordinary
file/directory and contained-parent controls; registered parent routing,
empty/stale allowlists, cross-root and escape rejections; read/edit target
controls; source and destination parent swaps using the existing barrier.
Keep barrier cases sequential and restore the hook on cleanup.

Use native temporary paths, separators, and rooted methods. On Windows skip
only the individual symlink fixture if `os.Symlink` fails specifically with
native privilege-not-held (error 1314). Other setup failures fail the test;
ordinary cases still run. Do not substitute Linux strings, cross-compilation,
or blanket platform skips for native correctness. Relative file and directory
links must remain valid native fixtures before their operation.

## Presentation, docs, and lifecycle

No layout, touch targets, scrolling, navigation, focus, labels, or responsive
composition changes. Backend entry/state correction serves both viewports;
real tracker and registered HTTP coverage satisfy the shared state/data mobile
exception. No browser/build/E2E is needed. Public `sessions-and-review.md`
already documents link identification and unchanged opening/saving; restoring
Delete/Rename entry semantics changes no user guidance, option, or terminology.
Public docs audit also covered `developer-tools.md`, root README and screenshot
catalog. No public-doc edit is planned.

No migration, new telemetry, polling, persistence, or restart flow is introduced.

## Requirement mapping

| Acceptance criteria | Design boundary | Planned evidence |
| --- | --- | --- |
| 001.1, 001.2 | Entry resolution, operation semantics | Real tracker leaf links and registered routes |
| 001.3 | Destination Lstat and entry no-op | Collision preservation and distinct-alias cases |
| 001.4 | Existing target resolver and parent routing | Ordinary, parent-link, registered-source, read/edit controls |
| 001.5 | Root admission and retained os.Root | Root/escape/cross-root/allowlist/barrier cases |
| 001.6 | Caller/transport, responses and notifications | Selected and aggregate HTTP requests plus immediate events |

All suffixes refer to `AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001`.
