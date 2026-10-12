---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-CREATE-CATALOGUE-001
---

# Executor creation catalogue publication design

## Purpose and boundaries

The executor system owns the accepted executor descriptor and shared owner/profile
catalogue. This design corrects only the publication following successful owner
creation through `/settings/executor/new`. Existing
[profile publication](profile-editor.md#normal-creation-catalogue-publication)
and [task selection](../../tasks/requirements/task-create-executor-default.md)
remain adjacent contracts. Reuse the existing owning-store boundary; no new
architecture, persistence, operational invariant or ADR is needed.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| AC-EXECUTORS-CREATE-CATALOGUE-001.1, .2 | Current catalogue publication |
| AC-EXECUTORS-CREATE-CATALOGUE-001.3 | Consumers and phone boundary |
| AC-EXECUTORS-CREATE-CATALOGUE-001.4 | Transport and failure compatibility |

## Components and actual creation contract

`renderSettingsRoute` in `apps/web/src/settings-routes.tsx` reserves
`/settings/executor/new` and directly renders `ExecutorCreatePage` from
`apps/web/app/settings/executor/new/page.tsx`. This is an active SPA route.
`ExecutorCreatePageContent.handleCreate` builds the existing payload with
`buildExecutorConfig`, selecting `local_docker` or `remote_docker`, name,
`status: "active"` and configuration. `getWebSocketClient` selects
`client.request<Executor>("executor.create", payload)` when a client exists;
otherwise `createExecutorAction` in `app/actions/executors.ts` POSTs
`/api/v1/executors`, preserving JSON headers and `cache: "no-store"`.

Backend `Service.CreateExecutor` in
`apps/backend/internal/task/service/service_resources.go` persists the owner,
publishes `events.ExecutorCreated`, notifies its existing save observer and
returns it. `httpCreateExecutor` returns HTTP 200 with `dto.FromExecutor`;
`wsCreateExecutor` returns the same DTO. `FromExecutor` supplies owner metadata
without attaching profiles. This flow does not automatically create a profile.
Do not change the backend, its observer, permissions or adapters.

`registerExecutorsHandlers` in `lib/ws/handlers/executors.ts` already filters
and appends created owners against current state, merges updates and removes
deleted owners. Its created-owner projection contains no profile list.
`registerExecutorProfileHandlers` in `lib/ws/handlers/executor-profiles.ts`
separately publishes nested profile create/update/delete notifications against
current state. Event payloads use canonical `ExecutorPayload` and
`ExecutorProfilePayload` from `lib/types/executor-payloads.ts`, re-exported by
`lib/types/backend.ts`. The create event can arrive before the response, but
actual delivery order is not guaranteed. These handlers remain unchanged.

## Current catalogue publication

Currently `handleCreate` captures `executors.items` from a subscribed render
before awaiting creation, then calls `setExecutors` with that earlier array.
Rerendering while the request is pending does not replace that invocation's
closure. Acceptance can therefore discard new owners/profiles, restore deleted
ones and overwrite surviving entries' values.

Acquire the owning `AppStore` using existing `useAppStoreApi` from
`components/state-provider.tsx`. After the selected transport accepts, read
`store.getState().executors.items` and synchronously call the existing
`setExecutors` action with the current entries filtered by the accepted ID,
followed by the accepted descriptor. No await separates this read and write.
Remove the captured catalogue and setter subscriptions as publication inputs.
Do not acquire a global store or reuse the similarly named Kubernetes upsert:
its replace-in-place order differs from this route's existing creation order.

All unrelated entries retain their current object values and relative order;
only the accepted ID is filtered and appended last. A creation notification
for that ID before acceptance is replaced by the accepted response, leaving one
descriptor. Preserve the exact accepted descriptor rather than manufacturing
profiles or merging speculative same-owner fields. This package claims no
same-created-owner profile retention. Any proposed change there requires an
independently authored causal RED and ordinary controls before implementation,
then an explicit bounded design adjustment. No timestamp or lifecycle framework.

## Transport and failure compatibility

Keep type defaults, type changes, seeded persisted names, remote TLS/token
configuration, payload fields, WS preference and POST fallback intact. Preserve
the current `try/finally`: successful publication precedes real
`router.push("/settings/executors")`; `finally` clears `isCreating`. Keep
Cancel and the pending disabled Create control unchanged.

Rejection occurs before publication and navigation. The existing rendered
click handler has no catch, so its rejected promise is unhandled. This design
does not introduce error handling. Do not swallow that failure in a test and
label it a rendered failure PASS. An explicitly awaited `createExecutorAction`
rejection, if examined, is transport-only evidence and proves no rendered
failure or finalization behavior. Successful rendered controls must complete
with zero unhandled errors.

## Consumers and phone boundary

`task-create-dialog-computed.ts` and `task/new-subtask-dialog.tsx` flatten current
owners' profiles, filling missing `executor_type` and `executor_name` from the
owning executor. They use real `useExecutorProfileOptions` from
`task-create-dialog-options.tsx`; current provider/capability eligibility is
retained. `ExecutorProfileSelector` in `task-create-dialog-selectors.tsx`
renders the resulting options as a desktop combobox or existing touch drawer.
No selection defaults, task admission or launch changes.

The create form, listing route, settings scroll owner and selector presentation
remain intact. The shipped selector's `TouchExecutorProfileSelector` is the
nearest touch exemplar; `components/kanban-with-preview.tsx` illustrates direct
phone navigation for settings destinations. Desktop and phone share this store
mutation. Under mobile-parity's explicit state/data-normalization exception,
real rendered route/store/options/selector integration evidence is sufficient:
no new mobile Playwright, geometry audit, UI sketches or browser build is
required. Reassess if composition, touch, scrolling, navigation or breakpoint
behavior changes.

## Verification boundary

Independently author
`app/settings/executor/new/executor-create-catalogue-publication.test.tsx`
and its `*.test-helpers.tsx`. Keep the real route resolver and page,
`StateProvider`/`createAppStore`, action adapter, WS client/connection selection,
registered executor/profile event handlers, SPA router, owner metadata fallback,
options hook and actual selector intact. A small test host may subscribe to
the real pathname and render `renderSettingsRoute` plus the real downstream
selector. Only external fetch or the selected client's request is held.
Canonical typed event fixtures must contain all required owner/profile fields,
including `is_system`, valid executor types, both scripts and deterministic
timestamps. Do not substitute a fixture for a production projection or bypass
an event with a direct test-only catalogue replacement.

Use ordinary accepted HTTP and WS controls for exact payload, adapter/action,
pending state, accepted descriptor once, choices and committed listing route.
Use held HTTP and WS acceptance with unrelated owner/profile addition, owner
removal, survivor metadata/profile updates and profile removal. Assert the
live pending catalogue and actual rendered selector before settlement, then
exact membership, order, values, fallback labels, eligibility and routing after
settlement. Separately deliver the accepted owner-created event before response
without inventing profile creation; assert accepted values and final position.
The preservation regression must fail on old source for the catalogue-loss
reason; uniqueness controls alone are already expected to pass there.

Restore connection/history/navigation state, settle held successful promises,
unmount providers/portals and drain owned timers on every exit. Never read,
copy or replay ROOT's protected candidate source. The allowed qualification
receipt is supporting evidence, not the permanent regression implementation.

## Documentation impact

Public executor setup/profile instructions and screenshots remain accurate;
no public route, field, command, copy or interaction changes. This pair and its
linked implementation package record the internal correction. No telemetry,
dependency, generated-contract or persistence changes.

## Implementation plans

- [Preserve live choices during executor creation](../../../plans/executor-create-catalogue-preservation/plan.md)
