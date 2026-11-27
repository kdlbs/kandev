---
status: current
system: ui
requirements:
  - REQ-UI-FILE-MOVE-SELECTION-001
---

# File Move Selection System Design

## Ownership and boundaries

UI constructs move commands from Files selection. This design owns only that
construction, before the existing workspace mutation boundary. Workspaces owns
[entry mutations](../../workspaces/system-design/file-entry-mutations.md).
[Files reply freshness](file-browser-reply-freshness.md#file-move-settlement)
continues to own per-request settlement, tree publication, and reconciliation.
Neither adjacent contract is rewritten by this correction.

The missing selection contract merits one requirement/design pair. No ADR is
needed: normalization uses the active coordinator without a new API, authority,
ownership registry, or persistence boundary. Suppressing descendants after
dispatch cannot prevent filesystem effects; serializing all selected requests
still performs redundant child moves. Normalize before target construction.

## Active callers and consumers

- `file-browser.tsx:useDragAndDrop` records the native drag selection in
  `dragPathsRef` and calls `executeMoveFiles` on an admitted drop. It is the
  sole production caller of that coordinator.
- `file-browser-move.ts:executeMoveFiles` uses the current tree, computes
  targets, captures source nodes, clears selection, and dispatches independent
  `onRenameFile` calls with existing `Promise.all` settlement.
- `file-tree-utils.ts:computeMoveTargets` resolves inventory nodes and reserves
  collision names in input order. It has one production consumer, the move
  coordinator. `moveNodesInTree` has no production caller; leave it unchanged.
- Desktop `files-panel.tsx` and phone `task-files-panel.tsx` pass real rename
  callbacks into the same `FileBrowser`. `useFileOperations` invokes the
  production `lib/ws/workspace-files.ts:renameFile` helper with session and
  old/new path payloads to `workspace.file.rename`. Single-entry context-menu
  rename bypasses the move coordinator and is unchanged.

## Effective-root normalization

Within `executeMoveFiles`, after the existing tree/current-owner admission and
before `computeMoveTargets`, derive effective source paths from that same tree
snapshot. Resolve selected entries with existing `findNodeByPath`; collect all
selected paths whose inventory node has `is_dir` true before filtering. Retain
a source only when no selected directory is its strict segment ancestor,
using the existing canonical `/` path identities and the `${directory}/`
boundary. A directory does not cover itself.

The complete selected-directory set, rather than only previously visited rows,
makes descendant suppression independent of selection order and depth. A stable
filter preserves independent-root order. Keep the normalization local to the
active move module, inline or in a private helper as needed for existing lint
limits. Do not export a test-only helper or modify unused tree-moving utilities
for symmetry. Missing source nodes retain `computeMoveTargets`' existing skip
behavior; this repair adds no new invalid-path, selection, or lifecycle policy.

Feed only effective roots to `computeMoveTargets`. Collision reservations,
captured nodes, requests, and affected-path reconciliation consequently all
describe that root list. Existing directory subtree prefix updates carry loaded
descendants under the chosen directory basename; the backend directory rename
carries actual descendants, including unloaded and unselected entries.

## Settlement and backend compatibility

Keep the current individual result handling, source-node guards, latest-tree
functional updates, ticket invalidation, final refresh, and toasts. Independent
roots retain `Promise.all` concurrency and invocation order. Covered descendants
have no request or individual outcome to fail. No new transaction, retry,
rollback, backend route, response field, or operation policy is introduced.
Self/descendant drop checks stay at the existing drag admission boundary.

## Responsive behavior

The shipped phone exemplar is `session-mobile-layout.tsx`: bottom navigation
selects Files, `TaskFilesPanel` mounts the shared browser, and tapping a file
replaces the list with `MobileFileViewerPanel`; Close returns to Files. Desktop
keeps its Dockview Files pane and editor. Files owns its existing internal tree
scroll viewport; phone layout retains bottom-nav/safe-area clearance and touch
row actions. The curated mobile language's focused-content and shared-logic
patterns apply. No controls, breakpoint rules, geometry, copy, scrolling,
navigation, focus, or touch behavior change.

Native HTML5 handlers exist on shared rows; that is not evidence of a supported
phone touch drag gesture. The shared state/data-normalization exception in
mobile-parity applies: real mounted command regressions plus existing responsive
component checks satisfy this repair. Existing mobile viewer E2E documents the
shipped browsing value but does not prove directory drag. No new mobile E2E or
phone move capability is planned.

## Verification

`file-browser-move-selection.test.tsx` independently specifies command outcomes
and reuses the mounted settlement fixture's real browser, selection hook,
tree, providers, operations, and WS dispatch. Only the transport/DOM geometry
boundary is substituted. Specify literal expected command lists and root counts
from the requirement; never calculate expectations with production normalization
or a copied normalization algorithm. Folder-first and child-first must each
fail causally before production changes. Include nested/mixed/prefix controls,
Shift range, directory collisions, and existing individual outcome feedback.

The fixture `drag()` accepts a separately specified expected root count, with
`paths.length` as its existing default. Shift selection uses direct real events.
Every existing settlement case remains intact.
Request evidence proves client command construction, not backend disk hierarchy.

One desktop scenario in `e2e/tests/task/file-tree-drag-drop.spec.ts` uses
real Files selection and HTML5 events, actual WS replies, and real backend
directory rename. Resolve the actual task checkout from environment evidence,
not just the seed repository path. Assert source absence, intact selected and
unselected sentinel bytes at nested destinations, no flattened child, and the
single root request. Run a fresh pre-fix production build for causal RED and a
fresh final build for GREEN, one worker and zero retries. The
[work order](../../../plans/directory-covering-file-moves/task-01-normalize-move-roots.md)
owns the bounded commands, independent case matrix, and evidence ledger.

## Requirement mapping

| Criteria | Boundary | Evidence |
| --- | --- | --- |
| AC-UI-FILE-MOVE-SELECTION-001.1, .2 | Effective-root normalization | Mounted both-order/nested/Shift cases; desktop disk hierarchy |
| AC-UI-FILE-MOVE-SELECTION-001.3 | Stable filter and segment ancestry | Mounted mixed-root, prefix and independent controls |
| AC-UI-FILE-MOVE-SELECTION-001.4 | Root-only target construction | Mounted directory and reservation collisions |
| AC-UI-FILE-MOVE-SELECTION-001.5 | Existing admission and settlement | Mounted mixed outcomes; all existing settlement tests and DnD controls |
| AC-UI-FILE-MOVE-SELECTION-001.6 | Shared browser, existing phone composition | Responsive component suite; documented mobile state exception |

No schema, telemetry, storage, configuration, or public transport changes.
