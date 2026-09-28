---
status: current
system: ui
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
---

# Task Navigation Responsiveness System Design

## Purpose and boundaries

UI owns client read coordination and presentation during navigation. This design
uses existing WebSocket actions, Zustand domain state, and file-tree models.
It changes neither backend authorization nor workspace/session lifecycle.
The server remains authoritative for readiness, missing paths, and file data.

The [render-isolation design](task-surface-render-isolation.md) continues to own
row identities, virtualization, positive measurements, and scroll ownership.
Do not replace the virtualizer or increase mounted-row counts to conceal delays.

## Requirement mapping

| Criterion | Design section |
| --- | --- |
| `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.1` | Overview hydration |
| `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.2` | Shared session reads |
| `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.3` | File-tree retention |
| `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.4` | Progressive restoration |
| `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5` | Scope and stale-response protection |
| `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.6` | Responsive presentation |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `use-swimlane-render-data.ts` | Safe projection memoization, including absent snapshots |
| `lib/state/session-read-coordinator.ts` (new) | Store-scoped request ownership and invalidation state; no React imports |
| Session shell/commit/diff hooks | Resource-specific keys, authoritative result handling, readiness, subscription |
| Git-status handler and base-branch picker | Explicit invalidation of the current store's diff scope |
| `file-browser-tree-cache.ts` (new) | Bounded tree snapshots for recent valid contexts |
| `file-browser-restore.ts` | Progressive dependency-aware folder restoration |
| `file-browser-tree-loader.ts` | Active load ownership, readiness/retry lifecycle, publication |
| `file-browser-hooks.ts` | Restore retained state and preserve interaction identities |
| `file-browser.tsx` / `file-browser-load-state.tsx` | Available rows plus localized loading or retry feedback |

## Overview hydration

`useTaskProjectionCache` must require an existing cache entry before comparing
its fields. An absent cache and an absent snapshot are distinct from a cached
projection. Preserve comparison of every visible input, including hidden step
IDs and task filters. Prune removed workflows as today. A missing workflow may
use the current loading/empty presentation; it must not crash another lane.

## Scope and stale-response protection

Use a `WeakMap<AppStore, ...>` owner, following the existing preview-feedback
read pattern. Scope entries to the current authenticated identity, backend API
target, WebSocket client and connection generation, and workspace ID/context
generation. Capture these values at request start; check them before publishing.
A reconnect on the same client must also advance the scope. Do not depend on
React component lifetime to establish transport or authorization lifetime.

Keys within a scope include all request arguments and the relevant session,
environment, and restoration attempt. Invalidation generations belong to the
entry; advancing one must not create a second request slot for the same resource
while a read is outstanding. Tuple encoding must distinguish missing task
identity from a task ID, without delimiter collisions. A new scope never reads
the old scope's entries. Release obsolete
maps/listeners and suppress their outstanding completions. Existing domain
state reset rules still apply; a coordinator hit must not bypass them.
Session-based reads also validate the current session-to-environment mapping
before publication. A response for the previous environment must not be written
through a session-keyed store action into its replacement or retained as a
completed snapshot after that mapping changes.

Unsubscribing one consumer removes only its subscription. It must not clear an
in-flight flag needed by other consumers. Completion and `finally` cleanup
must compare the entry's promise/owner identity, so an old request cannot clear
a replacement. Transport timeouts continue to bound outstanding work.

## Shared session reads

The coordinator owns promises and initialization/invalidation metadata. Existing
Zustand slices remain the shell and commit result owners. Move the cumulative
diff's module-global cache/listeners/timers into this same store scope, retaining
its last-known-value behavior. Do not create a second general server-state cache
or migrate unrelated queries.

| Resource | Read identity and preserved semantics |
| --- | --- |
| `user_shell.list` | Environment + optional task ID + `include_parked: true`; a prior environment-only response cannot satisfy the task-scoped read |
| `session.git.commits` | Session + environment; track refetch generation on the entry and obtain an authoritative initial snapshot even if live events prepopulated the slice |
| `session.cumulative_diff` | Session + environment; track invalidation generation on the entry and preserve last known diff during readiness/refresh and the existing invalidation coalescing interval |

Starting an initial read is immediate. Another consumer joins it. Successful
initialization belongs to the shared scope, not a hook ref; mounting alone is
not invalidation. A failed read leaves the key eligible for the existing bounded
retry behavior, without starting a retry timer for every consumer.

An invalidation during an outstanding read records one pending refresh. After
that read settles, one owner drains it if the scope is still current and has
consumers. An invalidation during the follow-up may schedule another necessary
read; do not lose changes by treating coalescing as a permanent suppression.
With no consumers, keep the entry dirty for the next subscriber instead of
running detached refresh loops. Preserve terminal-session handling, commit
empty-result rules, and explicit shell mutation invalidation.

Session-scope transitions must prevent an environment-only result arriving late
from replacing task-scoped shells. Distinct backend inputs may require separate
requests, but the current scope decides which may publish to a shared slice.

Bound settled, unsubscribed coordinator entries to 32 keys per resource using
least-recently-used eviction. Active subscribers and outstanding requests are
not evicted into duplicate work. Remove settled promises, unused timers, and
listeners. This bounds newly introduced retention; existing store retention
is not silently redefined by this repair.

## File-tree retention

Retain immutable tree metadata, not file contents or DOM nodes. The cache owner
is the app store; the key includes the valid workspace/environment/reset scope
and the same auth/connection/recovery protections above. A newly selected
session may reuse tree data only when it resolves to that same scope; requests
still carry the current session ID and load owner.

Keep at most four inactive tree snapshots and 20,000 nodes in total across
retained snapshots, evicting least recently used entries. A tree exceeding the
budget remains usable as the active tree but is not retained on departure.
Constants are private implementation limits, with eviction tests, not settings.
Do not persist this cache to localStorage, sessionStorage, or the backend.
Existing sessionStorage expansion/scroll preferences keep their current owner.

A cache hit publishes before refresh starts; it does not mark a refresh as
complete. Cached content cannot override workspace-unavailable, failed-session,
or restoration-required states. Clear invalid context snapshots and release
references on owner changes. Never retain an obsolete completion after eviction
or context retirement.

## Progressive restoration

1. Publish a valid retained tree, if present, without resetting it to null.
2. Read the root using the existing depth-one API. Publish the authoritative
   root immediately; reconcile retained children only for still-present paths.
3. Normalize expanded paths with their ancestor closure. Schedule at most four
   folder reads concurrently across the active restoration, parents first.
4. Publish each successful merge against the latest owner tree, preventing two
   concurrent completions from overwriting each other's changes. Preserve
   unchanged subtree identities. Do not wait for the slowest sibling to show a
   successfully loaded branch.
5. Start descendants only after their parent proves they exist and are folders.
   Share the same per-owner/path outstanding read with manual expansion so a
   user action cannot duplicate a restoration request.
6. Mark restoration complete when eligible work settles. If one branch failed
   transiently, keep other branches and its remembered expansion, show retry
   feedback, and retry through the same bounded owner. Prune expansion state
   only for authoritative missing/non-directory paths and their descendants.

Every scheduling and publication boundary checks `TreeLoadOwner` including its
generation. A task switch stops scheduling obsolete descendants. Late results
from A before an A-to-B-to-A round trip cannot update the new A owner. Where the
transport has no cancellation, discard results rather than invent cancellation
semantics. Do not count uncancelable old requests as completed new work.

## Failure and recovery

An uncached root uses the existing full-panel loading state. Retained/partial
trees remain visible during refresh and recoverable branch failures; localized
progress/retry feedback must not replace the usable tree. Reuse existing
`task:loadingFiles` and `task:retry` copy where suitable. Authoritative workspace
unavailability still replaces file interaction with the existing recovery UI.

Preserve existing root readiness checks and bounded empty-root retry delays.
An empty authoritative root clears obsolete rows. Transport errors do not count
as authoritative empty results. Ready/terminal responses retain their current
domain-specific meaning in shell, commit, and diff hooks.

## Responsive presentation

Desktop retains Dockview Files beside the current task content. Phone enters
through the existing Files bottom-navigation action in
`mobile/session-mobile-layout.tsx`, using a focused full-height surface rather
than additional panes or a drawer. The task chooser continues to use
`mobile/session-task-switcher-sheet.tsx` for temporary selection.

Both compositions share the loader, cache, filters, and actions. The existing
Files viewport owns vertical scrolling; fixed toolbar/navigation keep current
dynamic-viewport and safe-area behavior. File opening is the row's primary
action. Secondary actions remain visible and at least 44px on touch surfaces.
Partial loading must not steal focus or reset selection/scroll on every merge.

## Persistence and security

There is no migration, new API, permission, environment flag, or persisted user
setting. The in-memory caches retain only already authorized tree/read data.
Namespace retirement and context checks are mandatory even on reused stores.
Do not log paths, diffs, credentials, or identifiers as new metric labels.

## Observability and validation

Deferred-response unit tests prove sharing, invalidation, retry, eviction,
scope retirement, and parent ordering without wall-clock timing assertions.
Browser tests hold selected folder responses and assert available rows remain
interactive; they also count same-scope reads without misclassifying legitimate
refreshes or message backfill as duplicates.

Use an isolated production build for before/after navigation traces. Report
click-to-task identity, first usable content, restoration completion, request
counts, main-thread work, and long tasks separately. Compare debug logging on
and off; logs alone cannot establish render cost. Investigate retained memory
with comparable quiescent heap samples, distinguishing backend Go heap, browser
heap, and process RSS. Do not infer a leak from RSS alone.

## Related decisions and designs

- [System Info query cache ownership](../../../decisions/2026-09-26-system-info-query-cache-ownership.md)
  scopes TanStack Query to System Info. This repair keeps the existing WS/domain
  ownership and does not extend that migration.
- [Task surface render isolation](task-surface-render-isolation.md)
- [Workspace read recovery](../../workspaces/system-design/workspace-read-recovery.md)

No new ADR is needed: this uses the existing store-scoped coordination pattern;
the bounded local cache choice and its alternatives are preserved here.
Per-hook ownership cannot coordinate multiple consumers, while a global cache
would weaken scope isolation. A framework migration would expand the repair
without resolving the resource-specific readiness and invalidation contracts.
