---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-OWNER-DELETION-001
---

# Executor deletion catalogue continuity design

## Purpose and boundaries

The executor system owns owner deletion and the resulting catalogue consumed by
task profile selectors. This design covers successful publication by the active
legacy executor settings page. It extends the established owning-store pattern
without changing [profile editing and refresh publication](profile-editor.md).
Owner deletion is a distinct lifecycle explicitly excluded from that pair;
there is no parallel UI-owned requirement or generic state redesign.

## Requirement mapping

| Acceptance criterion | Design section |
| --- | --- |
| AC-EXECUTORS-OWNER-DELETION-001.1 | Current-store publication |
| AC-EXECUTORS-OWNER-DELETION-001.2 | Consumers and viewport boundary |
| AC-EXECUTORS-OWNER-DELETION-001.3 | Active route and transport; Failure behavior |

## Active route and transport

`LegacyExecutorSettingsRoute` in
`apps/web/components/settings/legacy-executor-settings-route.tsx` renders
`ExecutorEditPage` from `app/settings/executor/[id]/page.tsx` for executor-only,
non-Kubernetes routes. Explicit profile routes and Kubernetes redirects retain
their separate behavior. `ExecutorEditForm` renders `DeleteExecutorSection`
only when `is_system` is false. The section's confirmation compares the exact
`DELETE_CONFIRM_TOKEN` value, `delete`, without translation, trimming or folding.

The section selects `getWebSocketClient()` at submission. A client requests
`executor.delete` with `{ id: executor.id }`; without a client,
`deleteExecutorAction` performs HTTP DELETE `/api/v1/executors/:id` through
the actual action adapter in `apps/web/app/actions/executors.ts`. A successful
HTTP 204 supplies no body. Keep connection selection, action, ID, headers,
cache behavior and body semantics intact. Do not introduce backend changes.

## Current-store publication

`setExecutors` replaces the whole array. Successful deletion publication must
therefore filter current catalogue state: filtering a pre-request captured array
would lose later additions, restore later deletions and revert survivor updates.

Acquire this component's owning store through the existing `useAppStoreApi`.
After either deletion transport succeeds, read
`appStore.getState().executors.items`, filter only the submitted executor ID,
and synchronously publish through existing `setExecutors`. No await may
separate current-state read and publication. Remove the section's captured
catalogue subscription as publication input; retain the page's subscribed
executor lookup and the existing subscribed action. The request remains bound
to its original provider store, never a global singleton.

Filtering preserves each survivor object, nested profile list and field
verbatim, together with current order. If a registered `executor.deleted`
notification already removed the target, publication remains a target-only
no-op over current membership. It neither reconstructs an owner nor rereads a
server list. Same-ID recreation arbitration is excluded. No helper, new store
action, listener, revision counter or retry framework is necessary.

`registerExecutorsHandlers` and `registerExecutorProfileHandlers` already apply
live events against their owning current store. They remain unchanged. A newly
created owner receives its profile through `executor.profile.created`, because
the owner-created handler does not attach a nested profile list from its payload.
Regression fixtures must model those actual registered event shapes.

## Failure behavior

The current `handleDelete` uses `try/finally` without a catch. Transport rejection
skips catalogue publication and `router.push`, propagates from the async click
callback, and executes the final pending/dialog cleanup. Preserve this behavior;
adding a toast, catch, retry or coordinator would broaden the repair.

React does not await the click callback's returned promise. A rendered rejection
test must not treat an unhandled rejection as a passing control or suppress all
test-runner errors. The bounded safe rejection control awaits the actual HTTP
action adapter with an external non-success response and explicitly observes
its rejection. This proves transport rejection only, not a rendered failure
flow. Source control-flow inspection records the unchanged publication/navigation
and `finally` ordering. Rendered rejection coverage is deferred unless a focused
existing harness can observe the original callback rejection safely without
replacing internal UI, callbacks or error handling. Report this limit candidly.

## Consumers and viewport boundary

Task creation and subtasks flatten current owner profiles, filling missing
`executor_type` and `executor_name` from the owner, then call the real
`useExecutorProfileOptions` in `components/task-create-dialog-options.tsx`.
The real `ExecutorProfileSelector` in `task-create-dialog-selectors.tsx` renders
the resulting options, retaining provider eligibility and the existing desktop
combobox or touch drawer. See the
[profile-editor consumer contract](profile-editor.md#consumers-and-compatibility).
Deletion changes no default selection or task launch behavior.

This repair changes state publication inside an existing component. Markup,
copy, route destinations, layout, touch, scrolling and breakpoint behavior
remain intact. Mobile-parity's state/data exception permits real rendered
route/transport/store/registered-event/selector integration instead of new
phone Playwright tests, UI previews or whole-app builds. Reassess before any
interaction or geometry change. Public executor instructions and screenshots
remain accurate; internal specifications/plans are the documentation change.
There is no new persistence, operational or architectural boundary requiring
an ADR or telemetry.

## Verification boundary

Independently author tests under `app/settings/executor/[id]/`. Mount the active
`LegacyExecutorSettingsRoute`, actual page/form, `StateProvider`/`createAppStore`,
settings-save provider and required real providers. Keep real action adapter,
registered executor/profile handlers, client-router/history, catalogue
subscription, fallback projection, options hook and rendered selector. Hold
only external HTTP fetch or the external WS request boundary. External Monaco
renderer/loader may be isolated if absent from the DOM environment, retaining
loader exports and documenting the isolation. No internal causal boundary may
be mocked or replaced with an options list or private-callback invocation.

Drive the real dialog using its actual localized accessible label and exact
confirmation token. Await causal transport admission before live events. While
deletion is held, register a new owner and its profile, remove a different
owner, and update a retained owner's fields/profile inventory. Assert the live
catalogue before acceptance. Accept HTTP 204 or WS acknowledgement, then assert
the real SPA return to `/settings/executors` and the actual opened selector:
retained and new eligible choices remain, removed and target choices are absent.
Compare survivor values, order, identity and profile membership as well as labels.

Include ordinary accepted deletion, exact-confirmation and system-executor
controls, target notification before acknowledgement, and the explicitly awaited
transport-rejection control. Fixtures use non-system eligible targets, valid
owner/profile relationships and real option eligibility. Restore connection,
history/guards and external stubs; settle every held promise, unmount and drain
owned timers on every exit. RED must identify catalogue/choice loss, never a
fixture-label or unhandled-error failure.

## Implementation plan

- [Preserve the current catalogue during owner deletion](../../../plans/executor-delete-catalogue-preservation/plan.md)
