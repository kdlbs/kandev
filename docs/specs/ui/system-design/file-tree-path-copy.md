---
status: current
system: ui
requirements:
  - REQ-UI-FILE-TREE-PATH-COPY-001
---

# File Tree Path Copy System Design

## Purpose and boundaries

The UI owns a node-scoped copy action for Files. It consumes existing tree and filename-search node identity, and the [Changes and Review copy design](copy-file-path-actions.md)'s clipboard primitives; it does not own filesystem paths or mutation authority. The latter design's repository-relative path contract applies to Changes and Review, not to Files' potentially prefixed node identity.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-FILE-TREE-PATH-COPY-001` | [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), [Responsive presentation](#responsive-presentation) |

## Components and contracts

`FileContextMenu` and `FileContextMenuItems` offer the desktop action even when no other node actions are configured. Include it in menu presence and separator decisions without changing existing actions. `FileTreeNodeTouchActions` offers it in the existing visible overflow menu; the tree passes its actual selected count to this surface independently of delete availability. `SearchResultsList` constructs a file node from each result and remains single-result scoped. The same `node.path` goes to `useCopyRepositoryPath`; no path formatting or second clipboard helper is introduced. Reuse `task:copyPath` and the existing icons and localization.

## Control flow

A menu selection invokes the shared hook once with the exact node path. `isBulk` derives from more than one selected tree path and suppresses the action on both surfaces; search results do not inherit unrelated tree multi-selection. Menu events remain isolated from row handlers, so selecting the action does not invoke opening, selection, expansion, add-to-chat, delete, rename, upload, or download. Preserve existing menu focus and dismissal behavior.

## Responsive presentation

Desktop tree and filename-search rows are keyboard focusable with a visible focus indicator; the existing context menu supports keyboard opening and selection. Desktop uses the existing context menu. Phone and coarse-pointer rows use the existing 44px visible overflow trigger and DropdownMenu, matching the shipped file-tree chat-context surface. The node remains the primary row target; Copy path is a secondary item alongside existing actions. The shared responsive menu supplies its inset, safe-area-aware presentation and scrolling. This short contextual choice does not warrant a new surface. Verify the trigger and item geometry, viewport containment, and no horizontal overflow in a rendered phone scenario.

## Failure and recovery

`useCopyRepositoryPath` delegates to `copyPathToClipboard`, which refuses C0/DEL and shows `task:copyPathWithControlCharacters`. It uses the existing Clipboard API and fallback for other paths. On exhausted failure the hook currently supplies no success or error toast; preserve that behavior and do not claim success. Do not alter either primitive.

## Persistence, security, and observability

The action changes only the browser clipboard; no server call, storage, filesystem mutation, or new telemetry is needed. The existing control-character guard applies before clipboard access. Paths must not be converted to absolute paths or logged by the new action.

## Verification

Focused component tests cover menu availability, exact paths, bulk suppression independent of delete availability, failure behavior, and row-event isolation. Routed Playwright tests exercise real isolated task workspaces and actual clipboard reads on desktop and phone, including tree file, folder, and filename search. See the [work order](../../../plans/file-tree-path-copy/task-01-copy-node-path.md).
