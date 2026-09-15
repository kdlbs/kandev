---
status: current
system: tasks
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-005
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-006
created: 2026-09-15
owners:
  - kandev
---

# Add sources to the current workspace

## Ownership and mapping

Tasks own environment identity and durable source membership. Runtime owns executor filesystem operations and live tracking.
This extends [attachment](attach-workspace-sources.md) and [Worktree placement](workspace-repository-placement.md).
REQ-005 maps to Toolbar and presentation. REQ-006 maps to Admission, Persistence, Materialization, and Failure behavior.

## Grounding and scope

At planning baseline `35eb5ab2b`, `file-browser-toolbar.tsx` has a CreateMenu (+) with New file and uploads and a separate WorkspaceActionsMenu with attachment.
The implemented flow moves attachment into CreateMenu and removes the repository-count check from service admission.
The host materializer prepares an owned task root; this cannot be reused unchanged for an arbitrary user-owned CWD.
Remote attachment already uses `MaterializeRepositoriesForEnvironment`. Shared source capabilities prohibit arbitrary remote host folders.

This design removes the repositoryless exclusion for the new idle flow. Legacy active-turn add-branch, source detachment, root expansion, and automatic native-session replacement remain outside this extension.
The earlier expansion package keeps its own recovery dependency; no operation here changes CWD.

## Toolbar and presentation

Move the source menu item and its focus-restoration ownership into CreateMenu. Keep New file, Upload files, and Upload folder in their existing order, then a separator and Add repositories or folders.
Do not use the current one-click New file fallback when attachment exists but upload does not.
Retain Open workspace folder in overflow. Keep FilesPanel and TaskFilesPanel wired to the same AddWorkspaceSourcesDialog.
Use the existing source row state, repository selectors, bounded desktop dialog, and full-height phone drawer.
The phone + choice uses the existing accessible menu primitive, not a second source form. The source form has fixed header/footer, one scrolling body, dynamic viewport height, safe-area clearance, and 44px minimum touch targets. Desktop controls use the shared normal density.
The curated mobile menu exemplar is `components/kanban/mobile-menu-sheet.tsx`; reuse its safe-area and focus principles with the existing source drawer.
Keep all copy localized, including disabled reasons and link-versus-copy explanations. Reuse existing placement cards for Worktree tasks; do not replace their three-choice contract.

## Admission and preview

Remove the zero-repository guard only together with a durable root-preserving materialization path.
Extend the existing preview endpoint to folder and mixed batches and Local/scratch and supported remote environments. Preview must not clone or register repositories.
Resolve effective executor binding and workspace identity on the server. Return supported source kinds, destination choices, executor label, unchanged CWD, entry paths, link/clone semantics, and reason codes.
Reuse the existing revision protocol and per-task source lock. Bind preview to environment identity, live roots, source inventory, source selection, and placement. Revalidate before side effects.
An established writable root is required. A task without an environment gets an actionable 'Start the task to prepare its workspace' state; do not create a different root from this dialog.
Reject divergent live CWDs rather than claiming unchanged-CWD attachment. Preserve existing all-session idle admission.
Local folder/scratch defaults to current_root; allow kandev_directory as an explicit alternative. Existing Worktree selector semantics remain unchanged. Remote repositories use current_root and show the exact executor-side destination.

## Persistence and reuse

Keep TaskEnvironment.WorkspacePath as root authority. Preserve the task's original starting-folder/scratch intent after the first repository is added.
Extend typed environment root-origin metadata only where existing fields cannot distinguish user-owned folder, managed scratch, and repository/parent roots. Use dialect-aware additive migrations and conformance tests, never reinterpret a user folder as an owned task directory.
TaskRepository and TaskEnvironmentRepo retain repository and physical slot identity. Store attachment paths relative to the explicitly identified environment root for this extension; distinguish this base from legacy task-folder-relative paths with a typed discriminator. Never reinterpret existing WorkspaceRelativePath values silently.
Extend TaskWorkspaceFolder with the same placement-base/destination identity where needed; retain its canonical host source path and file-only semantics.
Launch/reuse preparers must prefer persisted environment root and inventory over repository-count branches. Cover environment reuse after adding the first repository, including local and remote scratch launch paths.
Repository projections, MCP provider refresh, Changes, branch selectors, and file-link resolution update only after commit. Do not manufacture a primary outer repository.

## Materialization and executor matrix

| Environment | Repository addition | Folder addition | Root/process effect |
| --- | --- | --- | --- |
| Local/Local PC user folder | Existing local checkout via owned link; remote locator through existing authenticated host clone seam | Existing host folder via owned link | CWD unchanged; rescan only |
| Local scratch | Same Local semantics | Same live host folder semantics | Scratch root retained; rescan only |
| Worktree | Existing real nested worktree implementation | Existing host link semantics | Preserve established Worktree contracts |
| Runnable Docker, SSH, Sprites, Kubernetes | Executor-side clone through existing environment materializer | Host folder attachment unavailable; Upload folder remains a separate copy action if supported | Existing remote CWD retained; rescan only |
| Unknown, disconnected, or unimplemented executor | No mutation; explicit reason | No mutation | No local fallback |

Do not infer support from a broad 'remote' label or an enum alone. Check concrete provider materializer capability and agentctl connectivity. Saved repositories need a cloneable locator for remote use.
A host path is a path on the backend host, not necessarily the browser computer. Label this in the picker. Upload is the existing browser transfer flow, with no sync promise.
Folder links preserve existing live-folder semantics; they are not physical containment or a sandbox grant. Do not copy a source to bypass provider permissions.
Generalize the owned-link helper to identify a task-owned entry inside a user-owned root without claiming ownership of that root. Reject self-links, ancestor cycles, unsafe names, symlink parents, collisions, and existing unowned entries.
Only create exact owned children and an optional grouping folder. Never write ownership markers that authorize recursive deletion of the user's CWD.
Apply effective outer Git exclusion/staging checks to repository links and clones as well as nested worktrees. A root that is not a Git repository requires no Git initialization or exclusions.

## Failure behavior

Use one compensated batch across folder links, repository clones/worktrees, inventory, and runtime tracking. Keep source targets and existing children untouched.
If live rescan succeeds but persistence fails, restore prior source roots by rescan, never rebind. Remote filesystem compensation must also run when a later database write fails.
Delete only resources proven created by this operation; preserve edited or pre-existing data. Exact retries compare canonical source, destination, environment, and branch identity.
Keep credentials out of previews and errors. Validate task ownership and same-workspace repository references before returning source metadata.

## Evidence and implementation plan

[Current workspace sources plan](../../../plans/current-workspace-sources/plan.md) contains three work orders and desktop/phone previews.
Tests must prove user-folder preservation, scratch first-repository reuse, folder-only and mixed batches, remote clone and rollback, unknown capabilities, focus restoration, and Files/Changes visibility.
The implementation and verification results are recorded in the plan and work orders. The earlier
workspace-repository-placement package retains its separate explicit expansion and recovery gate.
