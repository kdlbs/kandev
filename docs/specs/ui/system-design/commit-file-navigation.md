---
status: draft
system: ui
requirements:
  - REQ-UI-COMMIT-FILE-NAV-001
---

# Commit file navigation system design

## Purpose and boundaries

This design adds navigation to existing commit data. It does not change Git, WebSocket, provider, persistence, or authorization contracts.
The paired [requirement](../requirements/commit-file-navigation.md) owns the presentation outcome.

## Requirement mapping

| Criteria                                 | Design section                       |
| ---------------------------------------- | ------------------------------------ |
| AC-UI-COMMIT-FILE-NAV-001.1, .8          | Shared header and caller adapters    |
| AC-UI-COMMIT-FILE-NAV-001.2, .3, .4      | Commit content and navigation        |
| AC-UI-COMMIT-FILE-NAV-001.5              | Repository identity                  |
| AC-UI-COMMIT-FILE-NAV-001.6, .7          | Responsive presentation and recovery |
| AC-UI-COMMIT-FILE-NAV-001.9              | Collapsible index                    |
| AC-UI-COMMIT-FILE-NAV-001.10 through .13 | Inline commit files in Changes       |

## Shared header and caller adapters

Extract the file identity and collapse presentation from `components/review/review-diff-header.tsx` into a proposed `components/diff/collapsible-file-header.tsx`.
Its inputs are path, optional repository/status metadata, additions/deletions, collapsed state, toggle callback, and presentation slots.
The slots accept the review checkbox, stale indicator, and caller toolbar. The component owns no review state, fetching, or mutation callbacks.
Preserve the review adapter's existing status/stat visibility, sticky offsets, test attributes, toolbar, and mobile path hierarchy.
`ReviewDiffHeader` continues to construct `FileDiffToolbar`. `FileDiffSection` retains collapse, lazy visibility, auto-mark, and review selection state.

The commit adapter supplies no checkbox or review actions. It renders `FileDiffViewer` with `hideHeader` beneath the shared header.
The commit adapter must retain copy, wrap, split/unified, local file opening, and applicable context controls.
Expose the presentational buttons already used by `useDiffHeaderToolbar` for direct reuse, with an explicit resolved path instead of fabricated `FileDiffMetadata`.
Keep the hook as an adapter for existing callers. A commit toolbar owns its controlled wrap/context state and uses `useGlobalViewMode` for the existing display preference.
Phone secondary actions use the existing DropdownMenu treatment, with the review mobile toolbar as the interaction exemplar.
Do not pass fake review sources or dummy discard callbacks to `FileDiffToolbar`.
Respect `DiffViewerResolved` provider capabilities: Pierre-only context controls stay absent for Monaco. Both renderers retain copy and display tools.
Monaco's existing fold-unchanged control uses `components/editors/monaco/use-global-folding.ts`; reuse that hook in its toolbar adapter.
Folding available Monaco content does not fetch local context and can remain available for GitHub-only commits.

This is a local component extraction within the existing UI boundary. No new public component API or architectural ownership boundary is introduced.

## Commit content and navigation

`CommitDetailPanel` and `CommitDiffView` currently duplicate loading and metadata composition.
Share the new loaded content component between them, keeping their existing loading, error, and desktop tab-title adapters.
Use the existing `useCommitDetail` result and path-sorted `FileInfo` entries for both index and body rows. Do not filter files by patch availability.
Read stats from `FileInfo.additions` and `FileInfo.deletions`. Never calculate them from rendered hunks or apply cumulative-diff budgets.
These fields are optional in `FileInfo`. Show an unavailable label for absent values, preserving explicit zero values without coercion.

The loaded content owns a collapsed-path set and a map of section refs. Its identity includes the active session and complete `CommitDetailTarget` tuple.
For GitHub, the tuple includes source, workspace, owner, repository, and SHA. For local commits, it includes source, repository, and SHA.
Reset the subtree on identity changes. Use per-instance refs and React-generated accessible IDs so two panels cannot target each other.
Path keys remain stable across same-target file refreshes. Newly returned files start expanded.

An index button removes only its path from the collapsed set. After React commits the expanded body, focus the file toggle with `preventScroll`.
Then scroll the section into the owning detail viewport. Use a repeated action token or callback scheduling so selecting an already-selected entry still works.
Do not interpolate file paths into CSS selectors. Respect reduced motion and cancel pending navigation on target changes or unmount.
Keep headers mounted while bodies are collapsed. No new virtualization is required.

## Collapsible index

The commit content owns an `indexExpanded` boolean, initially true, separate from the collapsed-path set.
A semantic button labelled with Files and the count controls the index region through `aria-expanded` and `aria-controls`.
Collapsing removes only the index rows from the visible layout and focus order. It does not collapse any diff body or reset navigation.
The same target/session identity resets this state. No threshold, auto-collapse rule, or persisted setting is introduced.

## Inline commit files in Changes

`components/task/commit-row.tsx` currently uses an interactive `li` to open details. Replace this with a list item containing sibling controls.
The main button contains provenance, SHA, message, and stats. It controls the inline file region.
A separate icon-only Open commit button uses the existing `onOpenCommitDetail` callback and keeps that accessible name. Keep it visible without hover, including on phones. Its fine-pointer size stays within the original compact row height; phone and coarse-pointer controls use 44px targets.
Keep existing local commit actions and their eligibility rules. Action click and keyboard events must not trigger expansion.
Use semantic nested lists without nesting action buttons inside the main button.

Each row owns expansion and retains its loaded child after first expansion, hiding it when collapsed.
Mount the proposed `commit-row-files.tsx` only after the first expansion so `useCommitDetail` cannot eagerly fetch the whole history.
The existing hook owns source-aware loading and request-generation rejection. Keep its data alive for the mounted row after collapse.
Only one detail request runs for that row while loading. Retry stays in the expanded region without navigating away.
The current API returns patches together with filenames; loading an inline list therefore has the cost of a full commit-detail request.
Do not render diff engines in inline lists. Do not add a new endpoint or global cache for this package.

Use `userSettings.changesPanelLayout`, already consumed by `changes-panel-timeline.tsx`, for live flat/tree rendering.
Reuse `buildChangesTree` and `useTree` hierarchy/sorting rules by extracting the pure builder from `changes-panel-tree.tsx` into `changes-file-tree-model.ts`.
The existing `ChangesTree` includes stage/unstage/discard/edit actions; do not mount that mutable tree for historical files.
Render inline historical files through the shared `FileRow` read-only presentation and the shared `TreeDirRow` directory control so row density, icons, status/stat layout, and phone filename hierarchy track dirty/staged Changes files. The read-only branch hides worktree actions and routes every file type to the historical diff. The inline adapter retains source-aware data loading and layout selection.
Changing layout preserves the loaded data and row expansion. Directory state is transient per mounted commit row.
Reuse the same target/session identity rules as the commit panel and include full source/repository identity in row keys, not only SHA.

Extend commit-opening callbacks with an optional file-navigation request, separate from `CommitDetailTarget` data identity.
Thread it through `changes-panel-repo-groups.tsx`, `changes-panel-timeline.tsx`, desktop panel actions/params, and `mobile-changes-panel.tsx`/`DiffSheetMode`.
The request carries a file path and fresh navigation token. Repeated clicks on the same file must navigate an already-open panel again.
Panel identity and fetch identity remain based on the commit, never on the selected path.
After detail loading, validate that the path exists, then use the shared expand/focus/scroll handler. A missing path leaves the full commit visible.
Opening details without a file request keeps the current position of an existing panel. A newly opened panel starts at its header.
Never route historical files through ordinary `OpenDiffOptions`, which describe cumulative/worktree sources rather than a single commit.

Phone composition keeps inline files within the Changes list's existing scroll owner. Use a two-line commit identity and a visible sibling open action.
The file list follows the same saved layout, with long paths contained and all controls touch-sized.
The Open commit action and file selection use the existing full-height commit sheet. Row expansion itself opens no overlay.

## Repository identity

Use `useTaskRepositories` from `hooks/domains/kanban/use-task-repositories.ts` for the attached repository count.
Resolve the task from the active session's task identity, with the existing active-task fallback.
For local commits, prefer non-empty `target.repo`, then the matched local commit's `repository_name`.
For GitHub commits, show `target.owner/target.repo`. Never substitute the workspace's primary repository for a selected target.
If a legacy multi-repository target lacks identity, show a localized unavailable label rather than inventing a repository name.
Keep repository identity visible even when optional author/message metadata is absent. Single-repository tasks need no extra repository label.

## Responsive presentation and recovery

Desktop keeps the dockview panel. Its `PanelBody` owns vertical scrolling. The flat index precedes the diff rows in the same scroll region.
Phone detail entry becomes Changes -> Open commit action (or inline file) -> `MobileDiffSheet` -> `CommitDiffView`.
The full-height sheet is appropriate for sustained diff reading. Keep its fixed close header and use one internal vertical scroll owner.
Remove nested commit scrolling when embedded in that sheet. Bound the sheet with dynamic viewport height and preserve bottom safe-area clearance.
Limit changes to the commit composition where possible, and rerun the existing sheet tests if shared geometry changes.

Phone index entries show filename first, directory second, and stats beside the filename. The complete path remains accessible.
Use the same hierarchy in file headers. Long paths truncate within their region, while the document stays within viewport width.
The nearest shipped exemplar is `mobile/mobile-diff-sheet.tsx`; `ReviewDiffHeader` supplies the compact identity and visible actions pattern.
`useResponsiveBreakpoint` owns the composition boundary. Keep desktop ordinary controls at 28px and compact toolbar controls at 24px.
Phone and coarse-pointer controls have at least 44px hit targets. Do not persist phone display choices over desktop preferences.

Loading and request errors retain the current spinner and retry behavior. Empty collections have no index.
Patchless files use the existing `task:binaryOrEmptyDiff` treatment. Non-empty binary patches remain delegated to the existing renderer.
All added labels use i18n and all five real locales. Preserve existing source restrictions and renderer error behavior.

## Persistence and observability

Collapse and selection state are local to the open view. There are no schema changes, new storage writes, new endpoints, or new telemetry.
Inline expansion lazily uses the existing detail request. Independently opened detail panels may make their own existing request.
Existing tests and rendered geometry checks provide evidence for this presentation change.

## Implementation plan

- [Plan and work orders](../../../plans/commit-file-navigation/plan.md)
