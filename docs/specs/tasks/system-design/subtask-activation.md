---
status: current
system: tasks
created: 2026-11-03
owners:
  - kandev
requirements:
  - REQ-TASKS-SUBTASK-ACTIVATION-001
---

# Subtask activation system design

## Ownership and boundaries

The [requirement](../requirements/subtask-activation.md) owns the child-creation
handoff. This design uses the existing SPA router, task store, and task-route
hydration. It adds no API, persistence format, or lifecycle authority.

The [parent-defaults contract](../requirements/subtask-dialog-parent-defaults.md)
explicitly excludes activation. The [UI navigation design](../../ui/system-design/task-navigation-responsiveness.md)
owns reusable route projections and session fallback reads. This capability
does not duplicate those contracts.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-SUBTASK-ACTIVATION-001` | Creation handoff, Rendered task ownership, Failure behavior, Responsive surfaces, Verification |

## Creation handoff

`apps/web/components/task/use-subtask-submit.ts` owns successful creation.
Keep `session_id` precedence over `primary_session_id` and the existing null
fallback. Keep the synchronous submit guard and the current create payload.

After a response with a session, close the dialog first. Use
`useRouter().replace(linkToTask(childId, { sessionId: childSessionId }), options)`
with `scroll: false`. Pass the router action into the existing private creation
helper. The existing `onNavigated` callback publishes the selection only after
the history commit and router notification. `setActiveSession(childId, childSessionId)`
already sets the active task, pins the session, and remembers it for that task.
Avoid an earlier independent `setActiveTask` publication.

`apps/web/lib/routing/client-router.ts` already notifies `useParams`,
`usePathname`, and `useSearchParams`. Its replacement path preserves history
position and supports navigation guards. Reuse that path without adding events
or changing the URL-only `replaceTaskUrl` contract for unrelated callers.
The existing [settings navigation decision](../../../decisions/0046-settings-route-save-coordinator.md)
retains guard ownership.

## Rendered task ownership

`useTaskPageData` in `apps/web/components/task/task-page-content.tsx` must resolve
the task before validating its selected session. Compare candidates with the
resolved `task.id`, rather than the independently mutable global active task.

Validate both the active selection and route/fallback candidates against
`taskSessions.items`. Preserve current precedence among eligible candidates.
All admitted candidates must have `task_id === task.id`. Missing ownership
waits for existing route/session hydration. It does not authorize a guessed
conversation or a new session. An ineligible candidate does not prevent a later
owned candidate from supplying the view.

An explicit route session is authoritative while its row is missing: the page
renders no conversation instead of substituting another session of the same
task. Automatic session ensuring is disabled while the route session is
unresolved, and when an owned session is already selected. A route session owned
by another task is equally ineligible and cannot trigger a new session.

An old parent view can remain mounted during child hydration. Its task/session
pair must remain parent-owned, or its session must be null. Once the child is
ready, its owned session supplies the view. Keep the existing route generation
and late-response fences. Do not gate merely on global selection equality,
because the route intentionally retains usable previous content during reads.

`TaskPageContentLive` passes this same `effectiveSessionId` to panel state,
agentctl, resumption, focus, and the inner view. Keep these consumers aligned.
The desktop Dockview layout applies the same ownership test to its global
active-session read, falling back to the guarded session prop when the global
row does not belong to the rendered task.
The backend ownership rejection in `task_operations.go` remains authoritative.
There is no backend change or error suppression.

## Failure behavior

A rejected create keeps the existing translated toast and submit cleanup.
A response without a session keeps its existing close-without-activation behavior.
Neither path navigates, changes selection, or synthesizes a launch.

A deferred navigation retains the previous selection until `onNavigated`.
Cancellation leaves the created child intact and does not repeat creation.
Proceeding commits the captured child identity. No success-path repair retries,
full reload, raw `popstate` dispatch, or disabled ownership validation are needed.

## Responsive surfaces

Desktop uses the existing New Subtask dialog and task workbench. Phone uses the
existing task actions menu, responsive `NewSubtaskDialog`, and
`SessionMobileLayout` from `task-layout.tsx`.
The curated exemplar is the focused task layout and its mobile task switcher.
Creation opens a primary destination, so direct child navigation fits this flow.

Keep the phone task header, conversation scroll owner, composer, and bottom
navigation. The dialog closes before the child surface opens. Existing safe-area
padding, viewport handling, and touch targets remain in their current owners.
The shared submit and ownership logic does not depend on viewport width.

## Verification

Use a real-router hook integration for successful submission and navigation
guards. Mock the create transport, not the router or link builder. Add a mounted
task-page case with a parent route and a child selection, plus delayed ownership
hydration. Assert every observed status/recovery task/session pair.

Extend the existing desktop subtask E2E and add a focused phone counterpart.
Capture the create response to identify the exact child and session. Observe
outgoing WebSocket requests before submission. Assert the child's task heading,
conversation, and exact URL before reload, then repeat after reload.
The [work order](../../../plans/subtask-activation/task-01-align-child-navigation.md)
defines the scenario matrix and commands.

## Implementation plans

- [Subtask activation](../../../plans/subtask-activation/plan.md)
