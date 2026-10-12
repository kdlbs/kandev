---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-SYMLINK-001
---

# Symlink identification design

## Boundaries and evidence

Workspace entry metadata is authoritative. The existing platform
[Git status design](../../platform/system-design/workspace-git-status.md) owns
observation, budgets and mixed-layer delivery; this addition preserves those
contracts. `streams.FileTreeNode.IsSymlink` already describes file-tree entries.
`FileContentResponse.resolved_path` is populated for readable direct symlinks,
and `buildFileEditorState` stores it as `FileEditorState.resolvedPath`.
Changes `ChangedFile` and editor chrome consume this identity for their indicators.
The existing [workspace symlink ADR](../../../decisions/2026-07-19-workspace-symlink-entries.md)
concerns storage maintenance, not editor access; it is not expanded here.

## Metadata delivery

Add optional `is_symlink` metadata to `streams.FileInfo` and
`FileChangeFacet`, and mirror it in the frontend backend/state types.
Use a nullable boolean in Go when unknown must be distinguished from false.
Populate each layer from its destination mode, using its source mode only for
an actual deletion. Index mode identifies staged entries; working-tree modes
identify unstaged entries and Lstat identifies untracked entries; HEAD identifies staged deletions and
index mode identifies unstaged deletions. Git symlinks have mode 120000.
Mixed-layer facets own independent values; their flattened compatibility value
matches the flattened status layer. Renames use the applicable old/new paths.

Use at most two batched `git diff --raw -z --no-renames --no-ext-diff`
reads (one with `--cached`) through the existing tracker Git execution boundary
and its timeout/admission controls. `workspace_git_symlink.go` parses source and
destination modes without relying on filename quoting or patch parsing. Never
start one subprocess per file. Do not rely on diff body
availability: binary, truncated and budget-skipped rows still need metadata.
Avoid following target contents for classification; retain existing workspace
path checks and cancellation. Handle type-change porcelain status where needed
without changing existing stage/discard semantics. On metadata read failure,
leave classification unknown while preserving ordinary status availability.

Carry metadata through Git-status serialization, store equality/comparators,
layer expansion and `mapToChangedFiles` into `ChangedFile.isSymlink`.
Include the value in change signatures where equality suppresses UI updates.
Never borrow metadata from a different repository, session, or historical view.

## Editor and presentation

For successfully loaded files, derive link identity from the existing nonempty
`resolvedPath`. Pass it through `FileEditorContentProps`, both editor providers,
viewer/preview header variants, and the mobile viewer header. Keep identity in
file state rather than performing component-level requests. Refresh/reload paths
must update or clear resolved metadata, including when content is unchanged.
Retain current dirty-buffer conflict handling and request-generation guards.

A small shared noninteractive indicator renders a link glyph with a translated
accessible Symlink name. Changes shows the glyph adjacent to the filename,
outside the hover-swapped stage/action slot. The editor shows glyph plus the
visible translated label. It never takes focus or adds a click action.
The Files tree consumes existing `FileTreeNode.is_symlink` and replaces its
file/folder icon with the shared link glyph. Directory expand/collapse controls
remain unchanged. Search results without entry metadata retain their usual icon.
Keep it shrink-0 while the path truncates. Use existing tokens and icon library.
Translate new copy in all five locales with the established Traditional Chinese
generator; do not expose the target path as part of this feature.

## Mobile parity

Use `mobile-changes-panel.tsx` and `mobile-file-viewer-panel.tsx` as the shipped
exemplars. The indicator is inline content in their existing surfaces. Preserve
navigation, scroll ownership, safe areas, and touch action sizing. Link identity
must survive row action visibility and long-path truncation on phones. There is
no new tooltip-only disclosure, drawer, or preference.

## Persistence and failure

No migration, new polling, or metrics. Metadata is part of the existing accepted
snapshot/read. Unknown metadata renders no indicator. Broken links can be marked
in Changes while retaining the existing editor error. Classification grants no
new filesystem access and does not resolve or read external target contents.

## Requirement mapping

| Criteria | Design | Evidence |
| --- | --- | --- |
| 001.1, 001.3 | Metadata delivery | Real Git fixtures, JSON round-trip, skipped diffs |
| 001.2 | Editor and presentation | Both providers, direct target, repository isolation |
| 001.4 | Presentation and mobile parity | Refresh tests, desktop/mobile rendered checks |

All criterion suffixes refer to AC-WORKSPACES-SYMLINK-001.
