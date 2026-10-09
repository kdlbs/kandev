---
status: current
system: ui
requirements:
  - REQ-UI-WORKSPACE-SIDEBAR-VIEWS-001
---

# Workspace Sidebar Task Views Design

## Ownership and current behavior

UI owns reusable personal presentation preferences. Workspaces own identity,
visibility and lifecycle. Retain backend-owned portability under
[ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md).
No new ADR is needed: the scoped preference and migration rationale fit this
vertical design and its requirement.

Current `internal/user/models.UserSettings` holds global `SidebarViews`,
`SidebarActiveViewID`, and `SidebarDraft`. `applySidebarViews` and
`applySidebarViewState` validate these in the user-settings CAS transaction.
`sidebar-view-actions.ts` serializes writes and rollback through one journal per
store; `useEffectiveSidebarView` reads one global slice. Boot hydration and
`lib/ws/handlers/users.ts` both project that same global state.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-UI-WORKSPACE-SIDEBAR-VIEWS-001 | Scoped settings, Migration, Client projection, Failure handling, Surfaces |

## Scoped settings

Add a typed `sidebar_views_by_workspace` map to backend user settings, keyed by
workspace ID. Each entry contains `views`, `active_view_id`, and nullable `draft`,
using existing view/draft models. Identity is `(user_id, workspace_id, view_id)`;
legacy IDs may be identical in independent workspaces. Keep the map inside the
existing users.settings JSON and revision CAS mechanism; no separate view table.

Add a single-workspace `sidebar_view_state` PATCH object containing a required
`workspace_id` and optional `views`, `active_view_id`, and nullable `draft`.
Omitted fields stay unchanged; explicit null clears only draft; an empty view
array normalizes through `DefaultSidebarViews`. Validate active/draft references
against that entry and enforce the existing 50-view bound per entry. Each CAS
retry re-reads settings, replaces only the target map entry, and preserves other
entries and unrelated settings. Clone maps and nested data rather than mutating
references: the existing shallow-before/DeepEqual change detector depends on it.
Do not expose replacement of the entire map through PATCH.

Wire a narrow workspace-access dependency into `internal/user/service` from
`internal/backendapp/services.go`, backed by the existing authorized workspace
listing/read path. Resolve the user with `settingsUserID`; a client cannot name
another user. Reject unknown/inaccessible workspace targets. Do not infer the
write target from globally persisted `WorkspaceID`, which another tab can change.
Project only accessible entries. A deleted or inaccessible workspace is never
an eligible display fallback or write destination; retained personal JSON data
need not be eagerly garbage-collected by this change.

Complete HTTP, boot and user-settings event payloads expose the scoped map and
canonical default state. Update DTOs, controller/handler request mapping,
`boot_state_routes.go`, and TS wire/boot types together. One frontend mapper
normalizes boot, HTTP and WS payloads. Defaults for a newly created workspace
come from the backend contract, not a browser storage fallback.

## Migration

Persist a sidebar workspace migration version alongside the map. Before the
first effective settings read or scoped write for an unmigrated account, obtain
one successful authorized workspace snapshot. In the user-settings CAS path,
seed missing entries from a deeply copied, normalized legacy snapshot; keep any
existing scoped entry. Commit entries and migration marker atomically. Retry
CAS against fresh settings; never mark migration complete after a failed
workspace read or storage write. An account with zero accessible workspaces can
complete with an empty map; later workspaces use defaults.

After migration, an absent entry resolves to the canonical default and is
materialized on its first mutation. Never lazily clone legacy views into later
workspaces. Preserve copied filter values including unavailable repository or
workflow IDs; existing filtering determines empty results. Normalize invalid
selection/draft references within each copied entry.

Retain legacy fields as migration input and read compatibility for old clients,
but reject legacy sidebar mutation fields after migration with a validation
error requiring refresh. Reject mixed scoped/legacy patches atomically. Other
user-settings patches continue working. Do not route ambiguous legacy writes to
a guessed workspace. Document this stale-client limitation in the work results.

## Client projection

Represent sidebar state as a map of workspace entries; derive the current entry
from `workspaces.activeId` with shared selectors. Migrate every direct
`sidebarViews` reader, including picker, editor, chips, filter popover, sync
bridge and effective-view hook. Do not maintain two writable copies of the
active entry. Hydration and initial `setWorkspaces` selection must use the same
selector semantics as explicit `setActiveWorkspace` changes.

Actions capture the workspace ID before mutation and carry it through immutable
payloads, queues, journals and rollback. Use per-workspace journals and queued
writes. A view ID alone cannot identify an asynchronous operation. Bind open
editor/confirmation state to its originating workspace and close on context
change; stale callbacks must no-op after that binding changes.

## Failure handling

Keep optimistic state and rollback local to the originating entry. Track pending
writes and deferred server entries per workspace so full settings broadcasts do
not overwrite optimistic state in another entry. Track the acknowledged server revision on each workspace entry. Apply the
scoped PATCH response through the same mapper used by boot, HTTP reads and WS;
ignore older or duplicate entry revisions even when the global settings
snapshot has not yet received that acknowledgement. Once a workspace queue settles, reconcile its latest
eligible server entry or rollback, preserving a newer local selection/draft by
the existing intent-aware behavior. Report failures using localized existing
sync-error presentation; retaining an error for inactive A must not rollback B.
Test A-to-B-to-A switches with queued successes and failures, not just immediate
switches. Same-workspace full-list edits retain existing conflict semantics;
this change guarantees that different workspace entries cannot clobber each
other. No browser storage writes or retry markers are introduced.

## Surfaces and accessibility

Keep desktop Tasks picker/editor and phone
`components/task/mobile/session-task-switcher-sheet.tsx` as the shipped exemplar.
Phone entry remains task-switcher or app navigation > Task views. The drawer's
fixed controls and internal task-list scroll remain; chips scroll horizontally
inside their row, with no page overflow. Reuse its safe-area and dynamic-height
handling, keyboard dismissal, touch controls and focus return. Workspace context
selects data in shared hooks rather than a separate mobile state model.
No new visible labels are required; any necessary error/empty-state copy follows
all locale and i18n checks. See plan previews for scoped contents.

## Verification

Backend tests cover normalization, one-time migration including restart/failure,
user/access isolation, partial PATCH and concurrent cross-workspace CAS.
Frontend tests cover all actions, hydration/live projection, initial selection,
missing workspace and delayed rollback. Desktop and phone E2E prove independent
create/select/edit/reload and workspace-bound overlay dismissal.

## Delivery

- [Plan and work orders](../../../plans/workspace-sidebar-task-views/plan.md)
