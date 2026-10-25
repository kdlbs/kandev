---
status: current
system: ui
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
---

# File Browser Reply Freshness System Design

## Purpose and boundaries

This is a focused supplement to [task navigation responsiveness](task-navigation-responsiveness.md),
covering publication of Files search, file-watch refresh replies, and file-move
settlement into the same current and retained tree. UI owns
the current view and request intent. Workspaces and the existing WebSocket APIs
remain authoritative for filesystem contents and authorization.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5` | Search ownership; folder refresh ownership |
| `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.3` and `.4` | Folder refresh ownership |
| `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.6` | Shared presentation |
| `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.15` and `.16` | File-move settlement; move verification |

## Components and responsibilities

- `file-browser-hooks.ts` owns debounced search and file-change subscription
  lifetime. Small local helpers may hold publication checks and tree merging.
- `file-browser-data.ts` supplies the existing reset key and
  `FileTreeCacheBinding` to search as well as the tree. The binding's
  `isCurrent()` check retains the existing scope, environment, and restoration
  authority; search does not create a second context registry or tree cache.
- `applyFileChanges` refreshes affected folders through `requestFileTree` and
  merges accepted children into the latest tree through a functional setter.
- `WorkspaceTracker.buildFileTreeNode` propagates requested-directory read
  failures through the existing error response. Descendant failures retain
  their partial-tree placeholder behavior.
- Existing content/header components render the hook state in both desktop and
  phone Files surfaces.

## Search ownership

Search intent advances synchronously on every input change, including clear,
and on close. Session, reset, or binding changes retire the prior owner before
its completion may publish; unmount also retires it. Clear pending debounce
timers on retirement. The displayed query/results/loading belong to that owner,
so a previous session's settled snapshot is not displayed in its replacement.

Capture the intent and owner when scheduling work. Check both, and the current
binding, before starting transport work and before every success, error, and
finally state update. An old finally must not clear a newer request's loading
state. Reject obsolete failures without reporting them as current search
failures. The current failure keeps the existing empty-result settlement.
Retain the existing 300 ms debounce and 50-result limit.

The transport need not support cancellation. Retired replies are unwritable,
including an A-to-B-to-A return and a query changed while its next debounce
timer has not fired. Closing/clearing leaves tree mode immediately usable.

## Folder refresh ownership

Each subscription owner has independent publication tokens per requested
folder path. Register all tokens before issuing that event's reads. A later
request for the same path supersedes the earlier token immediately, including
while either response is pending. Unrelated sibling paths never supersede each
other. A single event may contain both stale and current folder replies: accept
each current path independently rather than discarding the whole event.

At completion, and again inside the functional tree updater, require the
subscription owner, context binding, and folder token to remain current. A
fully obsolete batch changes neither tree nor load state. Retire tokens with
their subscription, and keep bookkeeping bounded to work needed for outstanding
publication, without retaining an unbounded history of visited file paths.

Apply accepted root children authoritatively, preserving loaded children of
still-present directory placeholders. Apply accepted subfolder children to the
latest tree with the same depth-one semantics: an absent children field at the
requested folder means empty; descendant placeholders preserve already loaded
subtrees. A late child reply cannot recreate a parent removed by a current root.
Do not turn a transient tree-read error into authoritative deletion.

At the filesystem producer, an `os.ReadDir` failure for the requested directory
(`currentDepth == 0`) returns a wrapped filesystem error and no root. The existing
GetFileTree/HTTP error response and WebSocket rejection keep that failure out of
successful refresh publication. A genuinely empty directory still succeeds
with omitted children. Failed descendant reads remain directory placeholders;
depth limits, path validation, and containment retain their existing semantics.

## File-move settlement

`FileBrowser.useDragAndDrop` captures selected paths from native `DataTransfer`
and delegates to `executeMoveFiles`. `computeMoveTargets` defines the exact
source/destination mappings, including existing collision-name handling.
`useFileOperations.renameFile` calls the production `workspace.file.rename`
transport and converts both `success: false` and rejected requests to `false`,
with the existing per-file error toast. The requests are independent remote
mutations. Their request order and concurrency stay unchanged.

Remove the whole-selection optimistic move and captured-tree restoration.
Pending requests leave their rows at the last known locations until acceptance
or a workspace event establishes the new location. Keep selection clearing and
native drag state cleanup. Track each captured mapping's outcome independently;
wait for every dispatched request to settle before batch failure feedback and
final reconciliation. An unexpected callback rejection must not abandon accepted
or still-pending siblings. There is no retry or remote undo.

At each confirmed success, before queueing tree publication, supersede only the
existing pending tickets for that mapping's affected folders. Reuse
`changedFolders` and `nearestExpandedFolder` rules in `file-browser-refresh.ts`
through the actual subscription owner's guarded invalidation callback. Do not
issue reads at this boundary or retire unrelated tickets. A pre-acceptance root
or destination reply cannot erase the accepted row while a sibling remains
pending; a genuine later workspace refresh acquires fresh tickets and remains
authoritative. Keep the single final reconciliation after full settlement.

For a confirmed success, use a functional update against the latest tree. Capture
only that mapping's source node identity before dispatch. If it is still the same
node, its destination parent exists, and its exact destination is unoccupied,
remove that source and insert its renamed subtree at the captured destination.
Use `findNodeByPath`, `renameNodeInTree`, `removeNodeFromTree`, and
`insertNodeInTree`; never recompute collision names after dispatch. A directory's
loaded descendants follow the existing prefix-renaming behavior. Preserve node
identities outside the edited ancestry so one accepted sibling does not make
another captured directory appear replaced. If a newer refresh replaced, removed,
or relocated the source, or changed the destination,
leave that authoritative data untouched and rely on reconciliation. A failure
never inversely moves a node or restores any earlier tree.

Expose a narrow production `refreshChanges` callback from the existing
`useFileChangeSubscription` owner through `useFileBrowserTree`. Both actual
`session.workspace.file.changes` events and move settlement feed
`applyFileChanges` through this same owner and `FolderRefreshes` instance.
After all outcomes settle, submit the distinct captured old/new paths once;
existing nearest-expanded-folder grouping bounds the reads to affected visible
parents. Sharing tickets keeps a later event authoritative over a pending
settlement read. Do not create a second folder-ordering registry, root reload
loop, or optimistic overlay. Failed reads preserve the current tree; a failed
transport request cannot establish that the remote rename did not happen.

Reuse this owner's current/context guard for immediate success updates as well
as reconciliation, checking inside queued functional updaters. Retirement must
prevent a move's tree/cache publication into its replacement; this is immediate
Files ownership, not a global editor or writer lifetime redesign. Keep the
returned production callbacks stable and include them in the memoized tree
result. Move execution may live in `file-browser-move.ts`, imported by the real
browser, to keep the existing component below its file limit. Export no test-only
predicate, execution entry point, cache reset, or cache inspection API.

`useFileTreeState` remains the sole tree publication path. Its existing effect
writes the reconciled immutable tree into `FileBrowserTreeCache` under the
current binding. Never write the old snapshot directly to the cache or bypass
its scope/retention budget. [File-tree retention](task-navigation-responsiveness.md#file-tree-retention)
continues to own cache limits and restoration behavior.

### Move verification

Author permanent tests independently of the protected ROOT proof. Mount real
`FileBrowser`, `useFileOperations`, `StateProvider`/store, tree/virtualizer/cache,
`ToastProvider`, and `TooltipProvider`. Drive multiple selection and native
`dragStart`/`drop` with `DataTransfer` data; keep the real workspace-file transport
helpers and actual workspace-change subscription. Substitute only external
HTTP/WS endpoints, bounded DOM geometry, and the stable connection subscription.
Initial root and destination reads must be causally settled before moving.

Deferred request fixtures record exact rename payloads, accepted remote paths,
and current server-tree responses. Assert the accepted destination is rendered
by a real subscribed refresh before releasing the failing sibling, then assert
it stays there after settlement. Cover no-event mixed outcomes, both completion
orders and both accepted source positions, all-success/all-failure controls,
`success: false`, and transport rejection normalized by the real hook. Also cover
unrelated authoritative addition/removal/metadata and newer affected-path data,
failed reconciliation reads, and exact captured collision targets. Hold old
root/destination replies before acceptance, release them while the sibling is
pending, then reject final reads; accepted DOM/cache paths must survive while
an unrelated pending folder read and later authoritative updates remain valid. Observe the
actual retained result by remounting the browser within the same providers/store
while its next tree read is held; release all owned reads/timers in teardown.
Tests must not reproduce the settlement predicate or mock internal owners.

These are rendered client integration regressions, not real browser, filesystem,
or backend transaction proof. Existing DnD E2E protects native gesture reachability;
no new browser/build/E2E run is required for the state-only settlement correction.
The [single repair work order](../../../plans/preserve-successful-file-moves/task-01-settle-file-moves.md)
owns the exact scenario matrix and capped commands. No new ADR, schema, API,
copy, metric, persistence format, or request-concurrency policy is introduced.

## Shared presentation

Desktop Dockview and phone full-height Files keep their existing composition,
entry points, toolbar, scrolling, safe areas, and touch actions. This correction
changes shared async state publication only. Targeted hook tests and existing
desktop/touch search-result component tests satisfy the narrow state-normalization
exception in `/mobile-parity`; no breakpoint-dependent behavior changes.

## Persistence, security, and observability

No API, schema, setting, or persistence change is introduced. Use existing
context guards; do not add global request ownership or retain file contents.
Existing logging remains sufficient. No metrics or identifier labels are added.

## Validation and related decisions

Deferred-promise tests cover reversed search and tree completion, clear, close,
unmount, session/context retirement, stale failures/finally, independent
siblings, partially superseded batches, and loaded descendant preservation.
A removed-directory fixture covers requested read failures without relying on
permission enforcement, plus descendant/depth and genuine-empty controls. HTTP
client tests use the existing serialized response types; hook coverage proves
a rejected read preserves its loaded subtree while a successful empty reply
clears it and independent siblings publish.
The existing file-tree search E2E contract covers clear returning to tree mode.

The [System Info query-cache ownership ADR](../../../decisions/2026-09-26-system-info-query-cache-ownership.md)
does not authorize migrating Files to TanStack Query. This supplement implements
the existing local owner/generation guard pattern; it creates no new public
boundary requiring an ADR.
