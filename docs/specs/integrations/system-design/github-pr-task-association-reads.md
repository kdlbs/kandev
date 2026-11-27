---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001
created: 2026-10-06
owners:
  - kandev
---

# GitHub PR Task Association Reads System Design

## Purpose and boundaries

Integrations owns the reverse view of GitHub `TaskPR` associations used by the
GitHub browse page. Enforce eligibility at the existing `usePRKeyToTasks` read
boundary. Retain the existing cache, context identity and consumer API.

The [task sync design](github-task-pr-sync-coordination.md) owns task-scoped
request coordination. The [task summary design](../../ui/system-design/pr-task-status-summary.md)
owns task-to-PR disclosure, and [workspace read recovery](../../workspaces/system-design/workspace-read-recovery.md)
owns navigation collection refreshes. None owns this PR-to-task browse read.
This pair fits the existing Integrations boundary; no system README change or
new architectural decision is needed for a local application of existing scope.

## Requirement mapping

| Criteria for REQ-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001 | Design section |
| --- | --- |
| AC .1, .2, .3 | [Read eligibility](#read-eligibility) |
| AC .4, .6, .7 | [Reactive derivation and consumers](#reactive-derivation-and-consumers) |
| AC .5 | [Loading and failure](#loading-and-failure) |

## Components and existing contracts

- `apps/web/hooks/domains/github/use-pr-key-to-tasks.ts`: `usePRKeyToTasks(workspaceId)`
  returns `Map<string, TaskPR[]>`; `prKey` formats `owner/repo#number`.
- `apps/web/hooks/domains/github/use-task-pr.ts`: `useWorkspacePRs` loads the
  workspace collection through `listWorkspaceTaskPRs(workspaceId, { cache: "no-store" })`.
- `components/state-provider.tsx` binds `useAppStore` to one `createAppStore`
  instance. Nested providers reuse the parent store; isolation tests use sibling
  or separately mounted top-level providers, never nested independent stores.
- `lib/state/slices/github/types.ts`: `TaskPRsState` includes `byTaskId`, optional
  `workspaceId` and `workspaceContextGeneration`. Existing initialization in
  `lib/state/default-state.ts` normalizes compatible boot payloads. Reads add no
  fallback that infers ownership from record fields or request completion.
- `lib/state/slices/workspace/workspace-slice.ts`: `setActiveWorkspace` resets the
  kanban context on selection change. `resetKanbanWorkspaceContext` increments
  `workspaceContextGeneration` without clearing this GitHub association cache.
- `src/spa-routes.tsx` passes route `activeWorkspaceId` to `GitHubPageClient`.
  Its `AuthenticatedLayout` passes `workspaceId ?? null` to the hook and sends the
  map through `ResultsList` to `PRListBody`. That list reads the existing PR key
  and renders `PRRowTaskIndicator` through `PRRow`. The shared `TaskRowIndicator`
  owns empty, single-task and multiple-task presentation and navigation.

## Read eligibility

Before traversing `byTaskId`, require all of these conditions:

1. The requested workspace is non-null.
2. It equals `workspaces.activeId` in the owning store.
3. `taskPRs.workspaceId` equals the requested workspace.
4. `taskPRs.workspaceContextGeneration` equals the active
   `workspaceContextGeneration`.

Missing cache stamps or any mismatch produce an empty map. Do not mutate the
cache, clear context, wait for an effect, or add a record-by-record permission
filter. The stamped collection is the eligibility unit; existing writers and
backend response shapes remain authoritative for its contents.

## Reactive derivation and consumers

Subscribe through `useAppStore` to the existing `taskPRs` object, active workspace
ID and context generation. Include those values and the requested workspace in
the memo dependencies so changes to stamps alone invalidate the result, even
when `byTaskId` retains its reference. Use existing Zustand snapshots, without a
global map, new store field, registry, or copied association objects.

For an eligible cache, keep the existing inversion loop, defensive non-array
skip, enumeration order and `TaskPR` object references. Keep `prKey` and the
public signature unchanged. Neither the page nor row components need production
changes: their existing map lookup produces the localized empty indicator for
an absent key. No new labels, layout, touch targets or navigation are introduced.

## Loading and failure

Continue invoking `useWorkspacePRs` as today. Its `fetchedRef` remembers only a
workspace ID, so an unchanged workspace with a new generation need not schedule
a new request. That adjacent refresh issue is explicitly excluded. Likewise,
existing store setters apply supplied stamps; this design does not add a write
guard or repair obsolete publication. An obsolete stamped publication remains
unreadable, which guarantees safety without promising recovery.

Matching cached associations remain readable during pending/failed transport.
Publishing current-context data makes it readable; an empty current response
removes the reverse entries. Pending or failed reads never authorize stale data.
No HTTP, WebSocket, credential, permission, database, profile or setting changes
are required. This frontend fence is not a new authorization boundary.

## Verification and mobile parity

Use the real hook, `useWorkspacePRs`, provider, composed store and row components
with only the workspace-list API transport partially mocked using deferred
responses. Exercise prop-only and store-only changes, stamp-only updates,
independent providers and A-to-B-to-A context changes. Keep real routing,
localization and tooltip support. Assert association identity/grouping and actual
empty/single/multiple controls, rather than a copied eligibility predicate.

Desktop and phone consume the same derived map and existing shared row. This is
pure state selection with no layout, copy, scrolling, touch or navigation change;
the mobile-parity pure-state exception applies. Targeted hook/component evidence
is sufficient; no browser, Vite build or Playwright run is justified by this fix.
The [work order](../../../plans/github-linked-task-workspace-scope/task-01-scope-reverse-associations.md)
defines causal regression cases and exact checks.

## Related decisions

- [Active workspace identity](../../../decisions/0023-active-workspace-cookie.md).
- [Workspace integration scope](../../../decisions/0030-workspace-scoped-integration-settings.md).
