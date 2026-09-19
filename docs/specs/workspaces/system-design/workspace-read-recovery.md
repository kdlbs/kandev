---
status: draft
system: workspaces
requirements:
  - REQ-WORKSPACES-READ-RECOVERY-001
---

# Workspace Read Recovery System Design

## Purpose and boundaries

Workspace context owns collection identity, refresh outcomes, and isolation.
The platform owns database availability and stats admission; see
[interactive read availability](../../platform/system-design/interactive-read-availability.md).
This design corrects route hydration; it does not alter task persistence.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-WORKSPACES-READ-RECOVERY-001 | Collection outcomes; Context and recovery; Navigation presentation |

## Collection outcomes

`useRouteData` in `apps/web/src/spa-routes.tsx` currently replaces failed workflow,
repository, and step requests with successful empty values. `hydrate` then clears
shared workflow identities. `useWorkspaceSidebarTasks` uses these identities to
filter every snapshot and its fallback, so tasks disappear.

Replace these fallbacks with explicit success/error outcomes per collection.
Only successful outcomes may write collections. Read retained values from the
current store after the request settles, not from a captured pre-request copy.
A successful empty list remains authoritative. Retain live task updates.

Apply the same outcome distinction to `apps/web/src/kanban-route.tsx`, whose
bootstrap also uses an empty workflow fallback. Audit the route-owned repositories
and steps in that flow. Do not expand into unrelated settings loaders.
Keep `useWorkflowsFetchEffect`'s existing failure-preservation behavior.

## Context and recovery

Capture workspace ID and `workspaceContextGeneration` before each request.
Use `isCurrentWorkspaceContext` before every success, failure, and finally write.
Identity reset and unmount cancel pending work. Scope status and retained data by
workspace; a failure in workspace B must not keep A's navigation visible.
An access denial follows existing authorization handling and is not treated as
permission to keep actionable cached context.

Add narrowly scoped workspace-context refresh status and action state to the
existing kanban/workspace store boundary. Track pending/error per collection,
with a generation token; do not add a second copy of workflow or repository data.
The shared status must reach desktop and phone even when the route loader unmounts.
Inspect `lib/state/slices/kanban/types.ts`, `kanban-slice.ts`, and workspace reset
paths before choosing field placement. Clear status on context reset and success.

Use `useForegroundRefresh` for coalesced visible-page recovery. Initial transient
failures get at most two scheduled retries, at 2 and 5 seconds after the preceding
failure. Only network failures and HTTP 429/502/503/504 are transient. Respect a
larger `Retry-After`; cancel scheduled retries when hidden or unmounted. Manual
retry or one foreground return starts a new bounded attempt. Never retry 401,
403, 404, parse errors, or aborted requests automatically. Coalesce timer, manual,
and foreground triggers so each collection has at most one active request per
context. A successful retry must make failed snapshots eligible for refetch.
`useAllWorkflowSnapshots` currently remembers a failed fetch key; clear only the
failed key or explicitly refresh after workflow recovery without clearing data.

## Navigation presentation

Show one inline status above the existing task list, not one notice per workflow.
When data exists, show retained rows with a refresh-failure notice and Retry.
Without data, show load failure and Retry instead of `No tasks yet`.
Disable Retry while an attempt is active. Use localized status copy, semantic
buttons, and a polite status region. Preserve selection, filters, expansion,
scroll position, and the existing task links.

Desktop uses `components/app-sidebar/sections/tasks-section.tsx`. Phone uses the
existing task navigation drawer, grounded in
`components/task/mobile/session-task-switcher-sheet.tsx` and
`components/kanban/mobile-menu-sheet.tsx`. Add the same status inside its scrolling
list body. Keep its header, dismiss behavior, single scroll owner, dynamic height,
and safe-area treatment. Retry uses 28px desktop controls and at least 44px touch
hit areas. Do not create a second drawer or change saved desktop preferences.

## Verification and compatibility

Route component tests reproduce failed hydration with real store state. Cover
mixed outcomes, successful emptiness, late workspace A responses, identity reset,
and repeated retry triggers. Browser tests seed tasks, enter Stats, intercept
context reads with 503, and assert retained navigation and recovery. Repeat using
the phone drawer and touch Retry. Existing workspace-switch isolation tests remain
required. No schema, HTTP response, or task-state changes are needed.
