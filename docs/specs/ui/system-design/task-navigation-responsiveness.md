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
| `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.7` | Task route presentation and essential hydration |
| `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.8` through `.11` | Mounted task-session fallback ownership |

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

Successful ordinary and script terminal creation publishes the returned shell
to the owning environment's domain store immediately. Local terminal tabs and
optimistic removal use that same shell set; correctness cannot depend on a
later mounting consumer issuing another list request.

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
not invalidation. A failed read preserves the resource's existing settlement or
retry behavior, without starting a retry timer for every consumer. Shell-list
failures settle `loaded` with the latest known shells (or an empty list), so
terminal synchronization can proceed without discarding cached terminals.

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
Shell and commit publication ownership is environment-scoped because those
slices own one result per environment. Cumulative diffs retain separate
publication ownership for each complete request key: two sessions sharing an
environment must both receive their own session-specific diff.

Bound settled, unsubscribed coordinator entries to 32 keys per resource using
least-recently-used eviction. Active subscribers and outstanding requests are
not evicted into duplicate work. Remove settled promises, unused timers, and
listeners. This bounds newly introduced retention; existing store retention
is not silently redefined by this repair.

Session/environment and restoration bindings retire synchronously on every store
transition, including transitions batched before React renders. Retired entries
cannot be revived when a session returns to a previous environment; the new
binding starts a fresh read while obsolete completions remain unwritable.

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
complete. Retain the remembered expanded branches, but discard loaded children
of collapsed directories when restoring the snapshot. Those directories fetch
current children when opened; a depth-one root refresh cannot validate them. Cached content cannot override workspace-unavailable, failed-session,
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
   successfully loaded branch. The requested folder's children are authoritative:
   an omitted children field means empty at that level, while depth-limited
   descendants may retain their loaded children. A null folder response also
   clears that folder's retained descendants before pruning expansion state.
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

## Task route presentation and essential hydration

The SPA task route uses the current workspace's existing task projection and
validated session records to render available content during navigation. Do
not retain or replay complete hydration bundles: messages, runtime state,
settings, and session epochs remain owned by their existing store slices.
Unknown or cross-workspace projections and unknown requested sessions follow
normal authoritative loading. Task removal/recovery boundaries remain in place.

Client route resolution fetches only the task and its owned session list, then
hydrates those essential slices. The full boot/SSR enrichment entry points
remain unchanged. Existing mounted domain hooks own message/turn backfill,
profile reconciliation, repositories, workflow snapshots, settings, and shells;
one slow optional resource must not delay other content. The existing full-session
reconciler also initializes missing persisted model/configuration state through
the shared boot hydration mapper; a model event already in the store wins over
that background response. Omitted messages/turns
must remain unloaded, never become fabricated empty histories.

Read-cursor capture remains gated until the fresh session list is hydrated.
Readiness belongs to the exact hydration snapshot, not just the task/session
route key: returning to the same route requires its fresh snapshot to hydrate.
Automatic session creation also waits for authoritative task/session hydration;
a lightweight projection is presentation data, not permission to launch.
Task details supplied by route resolution must not trigger a duplicate details
request. Reconnects received before route readiness retain one pending details
refresh and drain it once hydration completes. Foreground refresh and route-error
recovery remain available.

Task/session ownership is checked before cached presentation. The task projection
preserves workspace, repository, status, and recovery metadata. Projection and
session selection use one store snapshot. Without an explicit session in the
URL, authoritative resolution preserves the currently selected owned session;
it falls back to the primary session only if that selection is no longer valid.
Route request
cancellation and session hydration epochs reject obsolete navigation and live
session overwrites. There is no new persistent cache or backend API.

This section implements `.7` and applies the `.5`/`.6` isolation and responsive
contracts to whole-task presentation.

## Browser work before task paint

Cached route data must also avoid unnecessary synchronous browser work:

- Responsive consumers share one event-updated snapshot and one set of media
  listeners. Ordinary renders reuse that snapshot; the last unsubscribe clears
  it so a later mount reads the current viewport.
- File-tree rows initially use positive cached or estimated heights. Browser
  ResizeObserver entries provide actual row and viewport sizes, including later
  resizes and visibility changes. When ResizeObserver is unavailable, measure
  mounted rows synchronously and keep the library's viewport fallback. Preserve
  positive cached row geometry while hidden and retain the existing row window,
  selection and scroll rules.
- Pinned-pane enforcement reads container width once before changing constraints;
  each subsequent layout event still measures the current container.
- Spinner CSS provides initial motion; animation promotion runs after the first
  frame and cancels on unmount. Existing visibility and reduced-motion ownership
  remains unchanged.
- Default static Markdown reuses context-free parsed element trees in an LRU
  bounded by 128 entries and 512,000 source characters. Oversized messages bypass
  retention. Custom renderers and active text motion bypass this cache; task and
  file-link providers stay outside it so callbacks and diagram identity always
  belong to the current consumer. This caches pure rendering, not authorized
  task state, mounted views or component effects.
- Unopened task-create forms do not mount. After first use they retain the
  existing close, draft and focus lifecycle. Sidebar selection reaches unrelated
  rows as an unchanged local boolean instead of a changing global task ID.

Use deterministic regressions for those mechanisms and matched production-build
Firefox/Chromium measurements for the aggregate effect. Animation-frame DOM
readiness is a navigation proxy, not proof of compositor paint. Retaining complete
chat views was considered but rejected: its modest measured benefit did not
justify changing message refresh, composer and hidden-view lifecycles.


## Mounted task-session fallback ownership

This section extends the existing client-navigation isolation contract. Task
session membership and canonical selection remain task-system responsibilities;
this resolver only projects current task data. It is separate from the shared
shell/commit/diff coordinator described above and does not use that coordinator.

### Local resolver contract

`apps/web/hooks/use-task-session.ts::useTaskSession` returns `sessionId`,
`hasSession`, and `isLoading`. Its current-task subscription reads
`taskSessionsByTask.itemsByTaskId[taskId]`. Keep the marked-primary-or-first
store selection and its priority over fetched data. Keep `task.session.list`,
`{ task_id: taskId }`, the 10,000 ms transport timeout, and first returned ID
fallback. Do not infer primary status or validate membership from the minimal
fallback response, which contains only session IDs.

Keep one task-keyed fallback/request snapshot inside each hook instance, with
an owner task ID, nullable session ID, and loading status. At render time,
project fetched fields only when their owner matches the current non-null task.
A mismatched retained snapshot cannot supply either a session or loading state.
This guards the first new-task render before effects run; an effect-only reset
would leave that render exposed. The current store result always wins and
suppresses fallback loading. Preserve existing null-task falsy loading behavior
and return field types rather than introducing an interface change.

The existing effect owns each request lifetime. Its cleanup deactivates obsolete
callbacks on task change, store takeover, or unmount. Preserve that guard for
both success and failure, including loading settlement: a superseded request
cannot clear the new request's loading state. Starting and settling a request
write a coherent snapshot for its captured task. With no client, settle that
task's snapshot empty and idle. A successful empty response and a failure do
the same. No retry, global cache, new store action, API change, or shared owner
framework is introduced. Same-task retained data freshness is not redefined.

### Audited consumers and reachability

At source audit on 2026-10-06, `components/kanban-with-preview.tsx` is the sole
production caller of this singular hook. The plural `useTaskSessions` is a
separate hook and is outside this correction. `KanbanWithPreview` stays mounted
as `useKanbanPreview` changes task or closes to a null selection.

`usePreviewSessionFocus` prefers a matching workflow-focus request, then a
user-selected session, then `selectedTask.primarySessionId`, then this hook's
result. `useSessionSelectionReset` resets explicit selection on task changes.
Thus the hook leak reaches the fallback branch only when those higher-priority
sources are absent. `useSyncSelectedTaskActivity` forwards that result to
`setActiveSession` or `setActiveSessionAuto`; these existing kanban-slice actions
do not validate membership. `useUrlSync` also writes the result as `sessionId`
for the selected task. Those are reachable incorrect task/session projections.
No change to those consumers or store actions is required.

`TaskPreviewPanel` forwards the ID to `PreviewSessionTabs`, whose
`pickActiveSessionId` checks the ID against its own current session list and
falls back to that list's primary-or-first, or null when empty. Its loading
branch also withholds chat content until a list is available. This mitigates
visible conversation selection. The hook proof does not establish that every
preview displays another task's conversation, or that a backend action accepts
an invalid pair. Keep those claims outside the repair.

### Responsive and verification boundaries

The shipped mobile exemplar is `KanbanWithPreview`'s direct-navigation branch:
phones render `KanbanBoard` without the preview pane. The hook and focus effects
are still called before that return, so their task ownership remains shared.
There is no changed composition, touch behavior, scroll owner, navigation,
copy, or breakpoint rule. The mobile-parity state/data exception applies:
real-hook and actual-consumer component evidence plus this note replace new
browser/mobile E2E. Existing preview/layout mounting is preserved.

Use the real hook, `StateProvider` (which creates `createAppStore`), and real
store actions. Mock only WS transport for new ownership tests, with one ROOT-reviewed
Happy DOM viewport exception for the actual consumer fixture: override
`offsetHeight` to 600 and `offsetWidth` to 320 only for real elements matching
`data-testid="kanban-column-scroll"`. Preserve original getters for all other
elements and restore them after every test. The real virtualizer and cards
remain active; no global geometry or component/framework mocks are introduced. Deferred promises
prove settled Alpha to uncached Beta, null close to different-task reopen, and
obsolete success/failure without timers. A real `KanbanWithPreview` integration
must prove the reachable active-store/URL boundary while Beta's reads are held,
and the primary-metadata mitigation; it must not mock the hook, provider,
selection logic, or affected store actions. Current success, empty, failure,
no-client, loading, primary-or-first priority, store takeover, and independent
instances preserve compatibility. Predicate-only or source-string tests cannot
provide this evidence. No browser, build, E2E, full-suite, or backend tests are
needed for this state-only correction.

### Implementation plans

- [Task session fallback ownership](../../../plans/task-session-fallback-ownership/plan.md)
