---
status: current
system: ui
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
---

# File Browser Reply Freshness System Design

## Purpose and boundaries

This is a focused supplement to [task navigation responsiveness](task-navigation-responsiveness.md),
covering publication of Files search and file-watch refresh replies. UI owns
the current view and request intent. Workspaces and the existing WebSocket APIs
remain authoritative for filesystem contents and authorization.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5` | Search ownership; folder refresh ownership |
| `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.3` and `.4` | Folder refresh ownership |
| `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.6` | Shared presentation |

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
