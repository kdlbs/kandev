---
status: current
system: platform
created: 2026-10-06
requirements:
  - REQ-PLATFORM-TASK-SLEEP-INHIBITION-001
owners:
  - kandev
---

# Task Sleep Inhibition System Design

## Purpose and boundaries

Platform owns the install-wide sleep preference and host runtime service. This
counterpart completes the existing [requirement](../requirements/task-sleep-inhibition.md)
with the browser publication boundary for acknowledged saves. No counterpart
was present at design HEAD `72a840a860143c6b55e401fe27c92167415b1e4a`.
The [original implemented plan](../../../plans/task-sleep-inhibition/plan.md)
records the backend/native delivery; the
[refresh repair plan](../../../plans/sleep-inhibition-save-refresh/plan.md)
owns the browser correction. This is a local resource boundary, not a
shared request framework or new server ordering contract.

## Requirement mapping

| Criteria under REQ-PLATFORM-TASK-SLEEP-INHIBITION-001 | Design section |
| --- | --- |
| AC-PLATFORM-TASK-SLEEP-INHIBITION-001.1 through .8 | Existing service and consumers |
| AC-PLATFORM-TASK-SLEEP-INHIBITION-001.9 | Acknowledged-save publication |
| AC-PLATFORM-TASK-SLEEP-INHIBITION-001.10 | Current reads and failure behavior; Draft and shared-save integration |
| AC-PLATFORM-TASK-SLEEP-INHIBITION-001.11 | Store sharing and view lifetime |

## Existing service and consumers

`internal/system/sleepinhibition` owns typed install-wide persistence and one
native lease reconciled against authoritative working sessions. The existing
`GET` and admin-only `PATCH /api/v1/system/sleep-inhibition` return
`SleepInhibitionResponse`: saved settings plus runtime capability/activity/issue.
The API functions `fetchSleepInhibitionSettings` and
`updateSleepInhibitionSettings` in `apps/web/lib/api/domains/sleep-inhibition-api.ts`
are re-exported by `settings-api.ts`. GET remains retrying and no-store; PATCH
returns the reconciled response. Native lifecycle, storage, authorization, API
shape, default, and all runtime reconciliation remain outside this repair.

`useSleepInhibitionSettings` selects the settings-domain `sleepInhibition`
snapshot. `setSleepInhibition` sets response and loaded, clears error and
loading; `setSleepInhibitionError(true)` marks loaded; loading has its own action.
These existing actions must remain unchanged. Initial loading, 15-second
visible-document polling, visibility-triggered refresh, and explicit retry
remain in the hook. A hidden visited Runtime panel stays mounted via
`SettingsTabsPanel`; this repair adds no active-tab or cadence policy.

`TaskActionsSettings` in `general-settings.tsx` contains the standalone
`SleepInhibitionSettings`. The active route is
`/settings/preferences/task-behavior`, which renders `TaskBehaviorSettings` and
the same card with `withinGroup` in its Runtime panel. Legacy General Task
Actions routes redirect there. The grouped card reports load/save attention to
the tab. The root `StateProvider` owns the app store; nested providers reuse it.
`SettingsLayoutClient` keys `SettingsSaveProvider` by pathname, so route changes
can retire a card/save registry while the app store survives. Do not claim a
full General route mount from the supplied component proof.

## Store sharing and view lifetime

Use a small module-private WeakMap keyed by the actual `StoreApi<AppState>`
from existing `useAppStoreApi`, located in the live sleep hook file. A hook-only
save epoch would miss another hook's pending GET and a route remount on the same
store. An unkeyed module singleton would leak publication ordering between
independent stores. No new store fields, provider, endpoint, abort signal,
server revision, auth subscription, or global framework is needed.

Each store entry holds only an acknowledged-save generation and the current
read identity. Each hook binding has a distinct committed lifetime and its own
flight marker. Bind callbacks to that store/lifetime. Activate and retire the
binding in a layout effect, before passive cleanup, following the nearby
`useCoordinators` lifetime pattern. Cleanup clears loading only if this binding
still owns the current read. Passive cleanup removes the existing timers and
listener. StrictMode setup must admit a fresh read after cleanup; old read
identities never reactivate. A store A-B-A transition uses new binding identities
rather than store equality alone. No ownership is activated or retired during render. A speculative suspended
render must leave the visible committed binding valid. A callback produced by
StrictMode's first committed setup remains tied to that producing activation;
consulting only the currently active epoch when invoking it would wrongly let
it dispatch after replay. The returned action properties expose callbacks from the committed activation
through getters, so retrieving a callback captures that activation. Replay
exposes its fresh callbacks without reactivating a retained retired one.

Keep the current same-hook in-flight admission guard. Across distinct hook
instances, the latest admitted GET owns publication to their shared store;
an older sibling cannot overwrite it or clear its pending indicator. Sharing
is relevant even if the normal route renders only one card: route lifetimes
reuse the store and views can share a parent provider. Independent root
providers retain separate generations and read identities. No cross-tab or
cross-install ordering is promised.

A retired callback cannot dispatch a new GET or PATCH. Retired refresh is a
no-op; retired save rejects with the existing `SettingsSaveCancelledError`
control-flow type, allowing the actual save provider to decline leaving. Do
not fabricate successful saves. Already-admitted PATCHes remain store-owned
writes to their captured store: completion returns the actual response or
throws the actual error even after card unmount. A successful admitted PATCH
still advances that store's acknowledgement boundary and publishes its response
to that store, never to a replacement store. This preserves an actual saved
install-wide preference across close/reopen and avoids silently cancelling a
backend write. No new ordering policy for concurrent PATCHes is introduced.

## Acknowledged-save publication

At GET admission capture the store generation, read identity, and active
binding identity before calling the real API. Only a still-current read from
that binding with the same generation may publish success, error, or loading
completion. Each terminal path uses the same boundary. Clear the originating
hook's flight marker only when it still refers to that request; old finally
must not release a newer flight or shared loading owner.

Starting a PATCH does not advance the generation or retire a useful GET. After
`updateSleepInhibitionSettings` fulfills, advance the captured store generation
and retire all pre-acknowledgement read publication identities before calling
`setSleepInhibition(next)`. This acknowledgement is the boundary: a GET begun
before saving or during the pending PATCH is now stale. PATCH success clears
load error/loading through the existing response action even if an old GET
is still physically pending. Treat a flight from an older acknowledgement generation as retired at the
next refresh admission, so a post-ACK refresh can start immediately even while
that transport is pending; the old finally cannot clear the new request. Do not wait for or abort the old transport.

Reads which settle while PATCH is pending may still publish under the current
read rules; the later ACK becomes authoritative. A GET admitted after ACK
captures the new generation and can publish a fresh saved/runtime snapshot.
Changing the generation never alters the value returned by save. An older GET
cannot turn an acknowledged save into a load failure or a different baseline.

## Current reads and failure behavior

A failed PATCH does not advance the acknowledgement generation, replace the
saved response, clear the draft, or synthesize success. A still-current GET
may settle normally before or after that failure. The API rejection propagates
to the contributor and shared coordinator. Current GET failure retains the
existing response, marks load error/loaded, and releases only its own loading;
retry clears current error at admission and publishes normally on success.
Initial failure and retry keep the existing loading/error card behavior.
Retired success, rejection, and finally are publication no-ops, without
unhandled rejections or indefinite pending indicators.

## Draft and shared-save integration

`SleepInhibitionSettings` owns local draft/saveFailed and last-saved alignment.
The existing alignment preserves an edited draft on status refresh; save
snapshots the submitted value and only applies the canonical returned value
when the current draft still matches that submission. Keep those semantics,
including the current boolean contributor revision. Do not invent an edit
counter or same-workspace policy.

`SettingsSaveProvider.saveAll` snapshots contributors, blocks invalid or
parallel saves, awaits actual contributor promises, records failures, and
checks newer revisions/newly dirty contributors before `canLeave`. Preserve
this contract and registration ordering. Deferred real-provider tests exposed
a close/reopen gap after the hook correction: the store accepted the saved
value while a new card retained the previous toggle value. Capture the previous
saved baseline before scheduling draft alignment, rather than reading a mutable
ref inside its state updater. During an admitted save, retain the existing
submitted-value alignment and establish the returned saved baseline before
completing that alignment. The retry wrapper retrieves the hook callback at
click time, after layout activation, so reopening a cached load error can retry
immediately. This local card glue preserves permitted in-save
edits and boolean revision semantics. The common save provider, store and API
remain unchanged.

## Responsive and public documentation audit

This correction changes state publication only. Desktop and phone use the same
card/hook/save domain. It changes no composition, copy, control geometry, touch,
scrolling, navigation, or viewport-dependent interaction. The mobile-parity
data-only exception applies: targeted real component/provider tests satisfy
this repair without new Playwright coverage, browser runs, builds, or previews.
Existing settings UI remains the exemplar; no new ASCII layout is needed.

`docs/public/operations.md` already describes the current Runtime entry point,
persisted install-wide preference, default, host boundary and native limitations.
`README.md` and `docs/screenshots.md` introduce no contradictory refresh contract.
No public guide, localization, screenshot, or navigation change is required.

## Verification boundary

Independently authored deferred transport tests exercise actual API, hook,
StateProvider/store, card and save coordinator. Mock only exported
`fetchJson`/`fetchJsonWithRetry` and a necessary navigation boundary; preserve all
other client exports. Assert transport admissions and observable store,
control, attention, coordinator and caller outcomes. Do not mock the hook,
provider, store, contributor registry or internal ownership predicate.
The [single work order](../../../plans/sleep-inhibition-save-refresh/task-01-save-refresh-boundary.md)
defines causal interleavings, resource receipts and exact commands.

The accepted ROOT rendered proof establishes local stale GET success/error
after acknowledged save. It does not prove failed backend persistence,
cross-tab ordering or a full route mount. Preserve the original proof read-only;
new regression tests must not copy, import or replay it.
