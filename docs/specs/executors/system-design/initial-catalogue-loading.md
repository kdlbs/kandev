---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-INITIAL-CATALOGUE-001
---

# Initial executor catalogue loading design

## Purpose and boundaries

The executor system owns this initial publication contract. Existing
[profile-editor publication](profile-editor.md#current-catalogue-publication)
and [profile-card refresh](profile-editor.md#profile-card-refresh-publication)
have separate lifetimes and remain unchanged. This design changes only the
executor initial-read effect in `useSettingsData` and its bounded private adjacent helper
`initial-executor-catalogue-read.ts`. Existing store
actions, APIs, registered events, boot state and task selectors remain owners
of their current behavior. No architecture decision or public API changes.

## Requirement mapping

| Acceptance criteria | Design section |
| --- | --- |
| AC-EXECUTORS-INITIAL-CATALOGUE-001.1, .4, .6 | Admission and completion |
| AC-EXECUTORS-INITIAL-CATALOGUE-001.2 | Invocation-local publication fence |
| AC-EXECUTORS-INITIAL-CATALOGUE-001.3 | Consumers and mobile outcome |
| AC-EXECUTORS-INITIAL-CATALOGUE-001.5 | Failure and cleanup |

## Current source and actors

- `apps/web/hooks/domains/settings/use-settings-data.ts` admits
  `listExecutors({ cache: "no-store" })` for an enabled empty, unsettled
  catalogue. Its unconditional success `setExecutors(response.executors)`
  replaces live state; its catch clears the catalogue. Finally sets
  `settingsData.executorsLoaded: true`.
- `apps/web/lib/api/domains/settings-api.ts` uses the real HTTP client for GET
  `/api/v1/executors`. The full response includes nested profiles.
- `apps/web/components/state-provider.tsx` supplies the owning AppStore through
  `useAppStoreApi`. `setExecutors` in
  `apps/web/lib/state/slices/settings/settings-slice.ts` replaces all items.
- `registerExecutorsHandlers` and `registerExecutorProfileHandlers` in
  `apps/web/lib/ws/handlers/` publish executor and nested-profile updates over
  current state. Their created notifications can reach this same store while
  GET is pending; they need no changes.
- Backend `boot_state.go:addTaskDetailAgentsState` and web
  `lib/ssr/session-page-state.ts` intentionally leave executorsLoaded false on
  task/session boot. `task-create-dialog-state.ts` and
  `task/new-subtask-dialog.tsx` call `useSettingsData(open)`, making the defect
  reachable beyond the Settings page.

## Admission and completion

Acquire the current provider's store with `useAppStoreApi` in the existing
hook. Keep the enabled/agent-resource gate and existing effect dependencies.
Re-read the owning store's `settingsData.executorsLoaded` and
`executors.items` immediately before admission, avoiding an outdated rendered
snapshot when another effect has already populated this store. Loaded means
skip; populated means mark loaded through the existing fast path and skip GET.

For an empty unsettled catalogue, start observation before invoking the same
real no-store GET. Keep request count policy as it is: multiple consumers may
admit reads, but each observes the store independently. Do not add a registry,
deduplication, latest-read sequence, retry or agent-list migration. A prior
response's nonempty publication is an observed transition for still-pending
reads. All completions retain the existing loaded finalization, including a
failed or skipped publication and a successful empty response. Live arrival
may cause the populated fast path to mark loaded before the response settles;
loaded is not proof of exhaustive inventory or successful transport.

## Invocation-local publication fence

Capture the owning store and subscribe only for this admitted initial read.
The admitted catalogue is empty. A sticky `changed` flag becomes true as soon
as the store contains any executor, and never becomes false within the read.
This observation is sufficient: there are no owner/profile values or order to
change before the first membership appears. Empty-array replacement and
unrelated settings changes are semantically unchanged; neither invalidates.
After membership appears, all current owner/profile values, order, additions,
removals and restoration are protected by retaining the whole catalogue.

On success, synchronously test the sticky flag and current catalogue. Publish
the response through the existing action only if no transition was observed
and the catalogue remains empty. No await separates eligibility and write.
Otherwise publish nothing, retaining current inventory rather than merging
untrusted response-only rows or resurrecting removed ones. This conservative
rule can defer unseen response-only choices; it adds no freshness claim.
Observation requires only one boolean and unsubscribe handle, not an event
journal, deep comparator, timestamp, version or per-executor tombstones.

Do not tie disposal to the React effect cleanup: live membership changes the
effect dependencies and may rerun the effect before the held GET settles.
Disposing then would lose transitions. Already admitted work stays bound to
its captured store until settlement, as current behavior does. No cancellation,
navigation lifetime, auth scope or transfer to another provider is introduced.

## Failure and cleanup

Retain silent failure and loaded settlement. Avoid catch publication that
clears current inventory: leaving an untouched empty catalogue alone has the
same observable outcome and preserves any intervening live rows. Cover this
necessary companion to the success fence independently before changing catch.
Use `finally` to unsubscribe on success, failure and skipped publication and
finalize loaded. Stop observing before the publication/finalization where
practical; the eligibility check and write remain synchronous. Do not leak a
subscription if transport throws synchronously. Admitted work must dispose
even after consumer unmount; it must never publish into another provider.

## Consumers and mobile outcome

`task-create-dialog-computed.ts` and `task/new-subtask-dialog.tsx` flatten
current nested profiles with `executor_type: p.executor_type ?? executor.type`
and `executor_name: p.executor_name ?? executor.name`. Real
`useExecutorProfileOptions` in `task-create-dialog-options.tsx` retains
eligibility, disabled reasons and owner labels. Actual
`ExecutorProfileSelector` in `task-create-dialog-selectors.tsx` uses those
options and the selected draft ID. Preserving only the ID would leave a
placeholder if its option disappeared; verify both selected ID and usable
option/trigger. The new-session dialog resolves labels through this catalogue
and `useTaskExecutorProfile`, but has no task-create options picker.

Desktop and phone share the same state publication. Under mobile-parity's
explicit state/data normalization exception, real component/transport/options
evidence satisfies this package without new Playwright, builds or UI sketches.
Composition, markup, copy, touch, scrolling, routes and breakpoints stay as
shipped. Reassess the exception if implementation changes any of them.

## Verification boundary

Independently author permanent tests in
`apps/web/hooks/domains/settings/use-settings-data-executor-publication.test.tsx`.
Mount actual StateProvider/createAppStore, useSettingsData, real listExecutors
and HTTP adapter, registered executor/profile handlers, actual fallback
projection, useExecutorProfileOptions and ExecutorProfileSelector. Keep a
real React selection draft, select a live eligible option while GET is held,
then settle the successful old response. Assert held admission and live
state/trigger before settlement, current whole inventory/order/values,
options/metadata/eligibility and selected trigger after settlement. Isolate
external fetch only, plus platform capabilities needed by existing primitives.
Do not mock the hook, provider, adapter, registered events or selector logic.

The primary RED must independently reach the missing catalogue/option/trigger
assertions with ordinary initial-GET and populated-catalogue controls passing
and zero setup/unhandled errors. Additional bounded cases cover mixed
add/update/remove, add/remove back to empty, rejection with live updates,
empty success/failure, disabled-to-enabled, loaded-empty skip, unrelated
settings/no-op empty replacement, effect rerender and observation disposal.
Preserve provider isolation without designing new request-lifetime semantics.
Permanent tests must never be read from or copied from ROOT's protected proof.

## Related contracts and implementation

- [Existing selector defaults](../../../decisions/2026-08-01-repository-task-executor-defaults.md)
  retain their precedence; this repair keeps the catalogue feeding them usable.
- [One bounded work package](../../../plans/executor-initial-catalogue-preservation/plan.md)
  owns exact tests, checks, resource discipline and later delivery gates.
