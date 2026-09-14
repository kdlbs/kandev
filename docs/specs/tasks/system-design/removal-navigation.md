---
status: draft
system: tasks
created: 2026-09-10
requirements:
  - REQ-TASKS-REMOVAL-NAVIGATION-001
  - REQ-TASKS-REMOVAL-NAVIGATION-002
owners:
  - kandev
---

# Task removal navigation system design

## Context and boundaries

The task system coordinates local removal intent, task presentation, and
fallback selection. HTTP and WebSocket task state remain authoritative.
No backend, persistence, permission, or cleanup protocol changes are required.

Source inspection found these gaps:

- Desktop and phone delete handlers await `deleteTaskById` before
  `removeTaskFromBoard`; `useTaskCRUD` also waits before removing board rows.
- `useArchiveAndSwitchTask` pre-switches, but `switchOnly` retains the outgoing
  task when no candidate exists. Candidate validation and session loading await
  network responses while the old view remains mounted.
- Task WebSocket handlers clear selection and redirect independently of the
  action. A later response can attempt another navigation.
- `TaskPageContent` and `KanbanWithPreview` call `useEnsureTaskSession`, so a
  visual overlay alone does not suspend task effects.

This is source evidence, not a captured browser reproduction. Work orders add
controlled delayed-response regressions before production changes.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-REMOVAL-NAVIGATION-001 | Presentation boundary, navigation, entry points, mobile |
| REQ-TASKS-REMOVAL-NAVIGATION-002 | Operation state, reconciliation, failure recovery, tests |

## Operation state

Add a small browser-local removal state within the existing application store,
with pure transitions in a proposed `lib/state/task-removal.ts`. It survives
component unmounts and SPA navigation but is excluded from boot hydration and
saved preferences. Do not encode pending intent in `archived_at`, task status,
or session state.

Each operation has a unique token, action, workspace, removal-set IDs, pending
request IDs, per-target results, and an optional departure record. That record
captures the outgoing task, session, route/preview context, environment identity,
and the navigation revision owned by the operation. Keep overlapping targets
from issuing duplicate requests; a bulk batch owns its complete exclusion set
before the first request. Preserve current per-target partial-failure semantics.

Navigation intent has a monotonically increasing revision. Explicit task,
session, workspace, preview, or route navigation advances it; automatic writes
from this operation carry its token. Recheck ownership after every await and
before every selection, URL, layout, or rollback write. Comparing task IDs alone
is insufficient when the user leaves and returns to the same ID. Use existing
selection and routing choke points; do not scatter counters among menu callers.
The existing task-selection actions live in
`lib/state/slices/kanban/kanban-slice.ts`; `setActiveSessionAuto` distinguishes
automatic session selection from explicit pinning. Route changes use
`lib/routing/client-router.ts` and its navigation guard. Preserve unsaved-change
admission before departure or mutation. A cancelled navigation guard must issue
neither; check ownership again when a deferred navigation actually commits.

## Presentation boundary

The accepted action synchronously publishes removal intent before calling the
mutation API or awaiting candidate resolution. A subscribed boundary outside the
task's data/effect subtree replaces outgoing content with a neutral status.
Split the wrapper from the live content where necessary to respect hook rules.
Cover all task-dependent chrome and Dockview content, including session tabs,
composer, banners, and archived/error views; preserve app-level navigation.

Use the same gate in `TaskPageContent` and `KanbanWithPreview` /
`TaskPreviewPanel`. Do not merely hide a mounted workbench with CSS. Also gate
`useEnsureTaskSession` through its existing `enabled` option and a current
removal-state check at dispatch, so an already scheduled effect cannot issue a
new ensure request. Previously dispatched requests retain existing server
lifecycle handling; the client neither undoes them nor recreates sessions.

The pending gate covers route/initial-task fallbacks as well as activeTaskId:
clearing selection must not let initial props remount the outgoing task.
Keep a successful operation's departure guard until the destination commit has
been acknowledged. Request settlement alone must not reopen stale content.

## Navigation and request flow

1. Preserve existing confirmation and discard-consent admission. On acceptance,
   capture all explicit targets and cached cascade descendants. Close the action
   surface and publish the departure gate. Opening a dialog does none of this.
   If an existing unsaved-change guard applies, finish that admission first.
2. For a detail view, reuse candidate ordering and HTTP liveness checks from
   `use-task-removal.ts`. Restrict candidates to the captured workspace and
   exclude all batch targets and descendants before mutation can prune caches.
   Revalidate current eligibility at commit. Missing or ambiguous candidates
   lead to the overview; never synthesize a candidate from an ID alone.
3. For cascades, a candidate with unknown ancestry cannot be proven outside the
   removal set. Resolve its parent chain using existing task reads, with cycle
   detection; skip it on missing/error data. Do not assume cached descendants
   are the complete server tree.
4. Run the unchanged archive/delete requests while destination resolution
   proceeds. Do not delay outgoing protection or mutation on session hydration.
   A preview closes immediately and clears its task/session URL parameters;
   it does not auto-open an unrelated preview. Record context for conditional
   failure restoration.
5. Commit destination selection, URL, and environment layout through one shared
   coordinator, extending `useTaskRemoval` and the existing link/router helpers.
   Use SPA replacement for fallback, including the last-task overview. Preserve
   `performLayoutSwitch` and validate destination session ownership. Show neutral
   loading while destination sessions/details hydrate; no outgoing content.
6. Reconcile success and issue one localized success notification. Release the
   operation only after both request settlement and departure reconciliation.
   Later user navigation relinquishes automatic destination/rollback ownership
   immediately, while request bookkeeping continues until settlement.

Unselected targets get request bookkeeping and duplicate suppression without a
departure record. Their completion never changes selection or focus.

## WebSocket reconciliation

Continue applying every authoritative task/session event to caches, archived
lists, recent tasks, pins, and Office refetch triggers. Pending UI intent must
not discard lifecycle events or their data cleanup.

In `task-lifecycle-side-effects.ts` and its callers, let a matching local
departure own navigation and presentation teardown. Avoid an intermediate
overview redirect or selection reset that races its chosen destination. Events
without a matching local operation retain the existing redirect behavior for
`/t/:id`, `/tasks/:id`, and `/office/tasks/:id`. Local removal elsewhere must
not suppress an unrelated Office redirect. A late event cannot alter the next
task's session or revoke its selection.

## Failure recovery

Separate mutation outcome from destination-loading failure. A successful delete
with a failed candidate fetch is still a successful delete; use the overview.
For a definitive refusal, such as the dirty-worktree 409, release pending intent
and restore the captured task/session/preview only if navigation ownership holds.
Validate session membership and availability before restoring session selection.

For network failures with unknown mutation outcome, refetch the original task
through existing authenticated APIs. Restore only a task confirmed available
and in its prior archive state. If it is gone, newly archived, or cannot be
verified, retain a safe destination and show an error/retry notification. Never
unarchive, recreate, or launch as rollback. A newer explicit user choice always
wins, including selecting another session or leaving and returning.

Bulk results retain failed IDs for retry, reconcile successful IDs, and report
one batch result. Conflicts reuse the existing discard guidance. Avoid duplicate
toasts between the coordinator, `useTaskActions`, and bulk callers.

## Entry points and mobile composition

Integrate desktop sidebar and phone switcher actions, `useTaskCRUD` board actions,
`useSidebarMultiSelect`, archive-confirm hooks, PR/banner archive actions,
command archive, and task-action message delete. `useArchiveAndSwitchTask` can
remain a compatibility wrapper around the shared coordinator. Low-level API
functions remain transport-only. Quick Chat task deletion in `sessions-dropdown`
and session-only deletion are separate lifecycles and remain excluded.

Nearest mobile exemplar: `task/mobile/session-task-switcher-sheet.tsx`, with
visible action menus and the existing confirmation dialog. On acceptance its
sheet closes; the main task layout or overview remains the single focal surface.
Preserve current `useResponsiveBreakpoint` composition, safe areas, dynamic
viewport height, and one content scroll owner. The neutral status occupies the
existing content region without adding a new drawer or scroll container.
Shared operation/selection logic serves both viewports. Move focus to a status
or destination heading and prevent dialog focus return to a removed row.
Any new actionable recovery control has a phone/coarse-pointer target of at
least 44px; desktop controls retain existing density. All new copy is localized
in the five shipped catalogs; generate Traditional Chinese with the repo script.

## Verification strategy

Unit tests use deferred promises with the real store, not only mocked removal
hooks. Inspect state after each event and prove token/revision ownership,
deduplication, cascade eligibility, bulk partial failure, and conditional rollback.
Rendered tests apply early session/archive/delete events while HTTP remains
pending and assert the outgoing subtree stays absent, including ensure requests.

Playwright tests arm HTTP/WS capture before acceptance. Hold mutation responses
with an explicit test-controlled release, deliver real lifecycle events, and
inspect the UI before release. Observe outgoing-subtree mounts/DOM mutations
throughout the interval; final-only visibility assertions cannot prove no flicker.
Keep controls for cold archived views, unselected removal, and remote removal.
Use real isolated backend fixtures, desktop and mobile projects, and managed
production builds. Exact scenario mapping and commands belong to the plan.

## Related records

- [Archive confirmation](archive-confirmation.md)
- [Dirty worktree deletion](dirty-worktree-deletion.md)
- [Mobile task navigation](../../ui/requirements/mobile-task-navigation.md)
- [Implementation plan](../../../plans/task-removal-navigation/plan.md)

This local orchestration change needs no new ADR: its constraints and rationale
are fully captured by this design and its requirements.
