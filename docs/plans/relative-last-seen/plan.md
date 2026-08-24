---
spec: docs/specs/ui/requirements/relative-last-seen.md
created: 2026-08-16
status: done
---

# Implementation Plan: Relative Last Seen in Account Security

## Overview

Add a persisted per-user enum setting `last_seen_display` (`"absolute"` default | `"relative"`) and
a select on the Active sessions card of `/settings/account/security`. In relative mode the Last seen
column renders a live-updating, locale-aware relative label with the absolute timestamp in a
fine-pointer tooltip or a coarse-pointer touch drawer. The setting follows the established
`changes_panel_layout` pattern end to end: JSON-blob user settings on the backend, boot-payload +
WS-push hydration on the frontend, and the shared manual-save settings flow in the UI.

## Design decisions

- **Shared manual save.** The select edits a local draft. The shared `Save changes` action calls
  `useSettingsSaveContributor` and persists the draft. The shared `Reset` action restores the
  confirmed value. A failed save keeps the draft dirty so the user can retry.
- **Queued write with revision-aware confirmation.** The contributor save uses
  `createQueuedUserSettingsSyncWithResponse`. The draft lives in local component state and is
  never written into the store directly. The store's `userSettings.lastSeenDisplay` is the
  confirmed baseline, updated by exactly two ingestion paths: a successful PATCH response mapped
  with `mapUserSettingsResponse(response, current)` and applied through `setUserSettings` (which
  discards older revisions), and a newer `user.settings.updated` WS snapshot through the existing
  handler path (`mapUserSettingsData` + handler revision gate, unchanged per task 02). A save
  response reconciles the draft with the revision-gated store result, so a newer foreign snapshot
  wins. A failed save leaves the submitted draft in place.
- **Revision-gated draft reconciliation; no writer-id correlation or store metadata.** The backend
  publishes the settings event before the PATCH response and broadcasts a full snapshot to every
  subscriber. A newer local draft stays visible while Save is in flight. The unchanged WebSocket
  handler applies own echoes and foreign snapshots to the confirmed store. After a successful
  response, the component reconciles the draft with the revision-gated store result. A failed save
  keeps the draft dirty for retry. A writer-id scheme is not needed because the shared settings
  provider owns Save and Reset, and the store remains the confirmed baseline.
- **Default `"absolute"`** preserves current behavior for users who never opt in.
- **`formatRelativeTime` from `@/lib/i18n/formats`** (locale-aware, accepts `now`) renders the
  relative label; the shared `useNow` hook drives the live update only while the cell is in
  relative mode (mounted in a dedicated child component so the interval is never created in
  absolute mode), with an age-aware cadence: tick every second while the timestamp is under a
  minute old (second-scale labels need live updates) and every minute once the age crosses a
  minute.
- **Exact time disclosure** uses `@kandev/ui/tooltip` with `formatDateTime(last_seen_at)` on
  fine-pointer devices. Coarse-pointer devices use the shipped `useTouchDrawer` and Drawer
  pattern to show the same timestamp after a tap.

## Backend

- `apps/backend/internal/user/models/models.go`: add `LastSeenDisplay string` to `UserSettings`
  with `json:"last_seen_display"`; add `LastSeenDisplayAbsolute = "absolute"` /
  `LastSeenDisplayRelative = "relative"` constants and a `NormalizeLastSeenDisplay` helper
  (anything other than `relative` coerces to `absolute`).
- `apps/backend/internal/user/store/sqlite.go`: default `"absolute"` in `defaultUserSettings`; add
  the field to BOTH hand-built JSON codec paths (the model struct alone does not persist it):
  `marshalUserSettingsPayload` map entry, the `scanUserSettings` payload struct field, and the
  scan assignment with `NormalizeLastSeenDisplay` coercion. Guard the PATCH write against the
  read-modify-write lost-update race: `UpsertUserSettingsPreservingTaskCreateLastUsed` gains an
  `expectedRevision` parameter and its UPDATE gains `AND settings_revision = ?` (SQLite and
  Postgres paths), returning a revision-conflict sentinel when zero rows match. The conflict is
  distinguished from a missing user explicitly: after a zero-row conditional UPDATE the repo
  existence-checks the user row; missing → the existing user-not-found error, present →
  `ErrUserSettingsRevisionConflict` (both drivers tested). The service routes ALL full-blob
  writers through one bounded read-apply-CAS-retry helper (read → apply patch → upsert with the
  read revision → on conflict re-read and re-apply the ORIGINAL patch, ~3 attempts): both
  `UpdateUserSettings` and `ClearDefaultEditorID` (service.go:883-900 currently upserts once and
  would otherwise either fail to clear on conflict or bypass CAS and overwrite the setting), with
  exactly one event publication after the successful final write. The helper carries the IMMUTABLE
  `taskCreatePatch` extracted from the original request (service.go:198-202) as a separate input
  passed to EVERY CAS attempt, and treats a non-empty task patch as `applied` even when the blob
  is otherwise unchanged (a task-create-only PATCH must not be classified no-op and dropped); a
  task-create-only PATCH under a forced conflict survives the retry, merges with the winner,
  increments the revision once on its own successful retry (the winner already bumped R to R+1, so
  the final revision is R+2), and emits the snapshot event (existing public behavior,
  service.go:197-206 / service_test.go:1257-1283). Every upsert callsite is
  updated: the store interface, both driver implementations, the controller/service test fakes,
  and the direct sqlite_test.go calls (~31/96/151/1274/1323/1379, passing the revision from the
  preceding read; the existing concurrent test at ~150 asserts conflict-and-retry semantics).
  Without this, a concurrent omitted-field PATCH (app-status-bar/review save) that read the
  pre-PATCH row can serialize a stale snapshot over `last_seen_display` at a newer revision,
  which the frontend then accepts as truth.
- `apps/backend/internal/user/service/service.go`: add `LastSeenDisplay *string` to
  `UpdateUserSettingsRequest`; add `applyLastSeenDisplay` (trim, validate against the two values,
  reject otherwise) and call it from `applyBasicSettings`; add
  `"last_seen_display": models.NormalizeLastSeenDisplay(settings.LastSeenDisplay)` to the
  hand-built `publishUserSettingsEvent` snapshot map so the WS `user.settings.updated` payload
  carries the field to every other tab. NO writer-id plumbing: the publisher signature stays
  `publishUserSettingsEvent(ctx, settings)` (no direct test callsite changes needed:
  `app_status_bar_visibility_test.go:44`, `service_test.go:1312, 1333, 1354, 1377, 1399, 1417`),
  and pending-selection protection is entirely client-side (pending-write gating).
- `apps/backend/internal/user/dto/dto.go`: add `LastSeenDisplay` to `UserSettingsDTO`
  (`json:"last_seen_display"`), map it in `FromUserSettings` via
  `models.NormalizeLastSeenDisplay(settings.LastSeenDisplay)` (matching how every other enum-like
  setting is normalized at the API boundary, so a model assembled outside the store can never emit
  a non-canonical value); add `LastSeenDisplay *string` to the DTO `UpdateUserSettingsRequest`.
- `apps/backend/internal/user/controller/controller.go`: pass `req.LastSeenDisplay` into the
  service request. The controller test asserts `req.LastSeenDisplay` reaches the service request
  (the REST handler and the WS `wsUpdateUserSettings` handler share this one controller path,
  handlers.go:113, so one test covers both transports).
- `apps/backend/internal/backendapp/boot_state_routes.go`: map `lastSeenDisplay` in
  `mapUserSettingsState` via a small normalize helper (mirroring `changesPanelLayout(...)`).

## Frontend contracts

- `apps/web/lib/types/http-user-settings.ts`: `LastSeenDisplay` type
  (`"absolute" | "relative"`); `last_seen_display?: LastSeenDisplay` on `UserSettings` and
  `UserSettingsUpdatePayload`.
- `apps/web/lib/types/http.ts`: add `LastSeenDisplay` to the explicit `http-user-settings`
  re-export list (the established import surface; omitting it breaks type resolution for barrel
  consumers).
- `apps/web/lib/state/slices/settings/types.ts`: `lastSeenDisplay: LastSeenDisplay` on
  `UserSettingsState` (default `"absolute"`). NO client-only metadata field: pending-selection
  protection lives in the component (pending-write gating), so no writer-id/lastWriterId state
  exists to be wiped by `mapUserSettingsResponse` callers that build from a fresh default state
  (use-layout-settings, layout-preset-selector, use-ensure-user-settings, hydration).
- `apps/web/lib/ssr/user-settings.ts`: default `"absolute"` for `lastSeenDisplay` in
  `createDefaultUserSettings`; `parseLastSeenDisplay` (only `"relative"` accepted, else
  `"absolute"`); map `lastSeenDisplay` in `buildAppearanceFields` via `mapDefined`.
- The WS `user.settings.updated` handler in `apps/web/lib/ws/handlers/users.ts` picks the field up
  through `mapUserSettingsData` with its existing revision gate, UNCHANGED otherwise (own echoes
  are applied normally, so the store converges even if the component unmounts mid-write); add
  `users.test.ts` assertions (valid value applies, unknown value normalizes to `"absolute"`,
  omitted field leaves the current value, stale revision ignored), so the cross-tab
  producer→handler path is tested on both sides.
- `apps/web/hooks/use-ensure-user-settings.test.ts` (`makeUnloadedSettings`, lines ~47-110) builds
  a complete typed `UserSettingsState` and gains `lastSeenDisplay: "absolute"`; audit remaining
  typed fixtures for the new required field.

## Security page UI

- `apps/web/components/settings/account/security-settings.tsx`:
  - Read `userSettings.lastSeenDisplay` from the store in `SessionsCard`.
  - Add a labeled select (Absolute time / Relative time) in the card, above the table, with a
    discovery target id `ACCOUNT_SETTINGS_TARGETS.lastSeenDisplay` and self-documenting copy.
  - On change: keep the draft in local component state and mark the `SessionsCard` dirty. Register
    the field with `useSettingsSaveContributor`. The contributor's `save` sends a queued
    `createQueuedUserSettingsSyncWithResponse({ last_seen_display })` write. The contributor's
    `discard` restores the confirmed store value. A failed save keeps the draft dirty and shows
    error copy so the shared Save action can retry.
  - Apply successful responses through `mapUserSettingsResponse` into `setUserSettings` only
    (never a raw store write). `setUserSettings` discards older revisions, so a deferred PATCH
    response arriving after a newer cross-tab WS event cannot regress the value or revision. After
    the response, reconcile the draft with the revision-gated store result. A newer foreign value
    therefore wins when it supersedes the submitted response.
  - Extract a small `LastSeenCell` component: absolute mode renders `formatDateTime`; relative mode
    mounts a ticking `useNow` wrapper and renders a `formatRelativeTime(last_seen_at, now)` label.
    Fine-pointer devices use a focusable semantic trigger and a `Tooltip` with the absolute time.
    Coarse-pointer devices use an explicit tap trigger and the shipped `useTouchDrawer` Drawer
    pattern to show the same absolute time. The cell validates the timestamp once; when unparseable
    it renders an empty cell with no disclosure surface (guarding the `formatDateTime` content,
    which throws `RangeError` on invalid dates).
- `apps/web/lib/settings-discovery/catalog/account.ts`: add `lastSeenDisplay:
  "setting-account-last-seen-display"` target and a `kind: "control"` discovery definition. The
  SessionsCard's `SettingsCard` already registers the `sessions` target and accepts exactly one
  `discoveryTargetId` (settings-card.tsx:7-30), so the select's wrapper registers the new target
  separately with `useSettingsTargetRegistration(ACCOUNT_SETTINGS_TARGETS.lastSeenDisplay)` (from
  `settings-target-provider.tsx`) rather than replacing the card's target; add a discovery-target
  test asserting both targets register and reveal.
- i18n: new keys in `apps/web/src/locales/en/account.json` (+ pseudo/pt-pt/zh-cn/zh-hk/zh-tw
  mirrors). Use plain punctuation (no em dash).

## Tests

- **Backend:** `service` test for `applyLastSeenDisplay` (accept both values, reject invalid, nil
  no-op); a `publishUserSettingsEvent` test asserting the published event map carries the
  normalized `last_seen_display` (in `service/user_settings_cas_test.go`; the existing publisher
  tests at `service_test.go:1312-1417` stay untouched); store tests asserting `defaultUserSettings`,
  a JSON round-trip of the new field through `marshalUserSettingsPayload`/`scanUserSettings`
  (stored `"relative"` reads back as `"relative"`, stored unknown/empty coerces to `"absolute"`, a
  later PATCH updates the stored JSON); a `dto` test asserting `FromUserSettings` emits the
  canonical value for an unknown model value; a `controller` mapping test asserting
  `req.LastSeenDisplay` reaches the service request (the controller is a manual field-by-field
  adapter; an omission compiles and silently no-ops); `boot_state_routes_test.go` mapping test
  (relative passes through, unknown coerces to absolute); a barrier-based concurrent PATCH test
  proving an unrelated omitted-field write (started from the same initial revision) cannot revert
  `last_seen_display` — both fields are present in the final row and the retry loop merges the
  loser's patch; a concurrent `ClearDefaultEditorID` + settings PATCH test (the editor is cleared
  AND `last_seen_display` survives; TWO events are published, one per successful write with
  distinct revisions — the helper contract is exactly one event per successful final write, so two
  successful writes emit two events; a one-event outcome is only valid in the no-op scenario); a
  CAS-conflict no-op barrier test that FORCES callback re-evaluation (Clear's initial read
  completes seeing `old-editor`, its upsert is blocked; the PATCH commits `new-editor` at R+1 and
  publishes; Clear's stale upsert then conflicts, the retry fresh-reads the row, the callback
  re-evaluates with the immutable `old-editor` and returns `applied=false` → no Clear write/event;
  assert read count >= 2, `clearUpsertAttempts == 1` (the forced stale CAS invocation that
  conflicts) with `clearSuccessfulUpserts == 0`, `clearEvents == 0`, exactly one PATCH event —
  counters named to distinguish attempts from successful commits) PLUS a separate
  fresh-read no-op test (Clear starts after the PATCH committed; immediate no-op, no conflict);
  repo zero-row tests for both drivers (missing user → user-not-found,
  revision mismatch → `ErrUserSettingsRevisionConflict`); direct repo tests migrated to CAS and
  MOVED to new focused files (the unique-revision concurrent test becomes one success + one
  conflict; the stale-blob merge tests re-read the current revision or assert the conflict)
  because `sqlite_test.go`/`service_test.go` are over Revive's 800-line test-file limit and new
  code there fails the changed-file lint — new files: `store/sqlite_last_seen_cas_test.go`,
  `service/user_settings_cas_test.go`. The concurrent SERVICE tests use a dedicated synchronized
  fake (clones settings per read, models revision-conditional writes, records reads/upserts and
  events through synchronized state/channels, channel barriers for phase observation) — NOT the
  existing `recordingUserRepository`/`recordingEventBus` fakes, which share one mutable
  `getSettings` pointer, mutate unprotected fields, and append events unsynchronized
  (service_test.go:1741-1801), racing under `go test -race`.
- **Frontend:** `ssr/user-settings.test.ts` covers `parseLastSeenDisplay` and
  `mapUserSettingsData`; `users.test.ts` covers valid, unknown, omitted, and stale WebSocket
  snapshots. The `SessionsCard` tests cover the absolute default, relative labels, fine-pointer
  tooltip and keyboard focus, coarse-pointer drawer disclosure, live ticking, invalid timestamps,
  local draft state, shared Save and Reset actions, failed-save retry state, revision-gated PATCH
  responses, foreign WebSocket updates, and unmount/remount convergence.
- **E2E (desktop + mobile + cross-tab):** add `apps/web/e2e/tests/auth/relative-last-seen.spec.ts`
  (Desktop Chrome, `auth` project) grouped in `test.describe.serial` (single-shot `setupAdmin` +
  shared worker backend/DB require serial ordering), restarting the worker backend with auth on and
  its OWN `KANDEV_DATABASE_PATH` (auth setup is single-shot and worker DB is preserved across
  restart; without a dedicated DB the focused command collides with sibling auth specs), calling
  `setupAdmin` in `beforeAll` so focused test runs are independent, logging in, opening
  `/settings/account/security`, switching to Relative time, saving through the shared Save action,
  and asserting a relative label renders with an absolute tooltip on hover. Restore the original
  setting in `finally`, and restart the baseline backend in `afterAll`. The same file adds a
  two-context cross-tab scenario that arms `watchWs(pageA)` BEFORE `pageA.goto` (it only sees
  sockets opened after it is called) AND arms the subscription wait (`const subscriptionAck =
  watcher.waitForResponse("user.subscribe")`) before the same `goto`, then awaits `subscriptionAck`
  after navigation before tab B mutates (frames are not buffered and `waitForResponse` only records
  request ids once armed, so arming the wait before navigation is required to catch the ACK); the
  assertion is causal: tab B changes the setting, tab A observes the update via WS without a
  reload. Add `apps/web/e2e/tests/auth/mobile-relative-last-seen.spec.ts` (`mobile-chrome`
  project, named `mobile-*` so the project routes it) that spreads `...devices["Pixel 5"]` into a
  manual `browser.newContext` (manual contexts do not inherit project device options), asserts the
  resulting viewport width, uses its own DB + `setupAdmin`, operates the select with touch-native
  `tap()` on the trigger and option, saves through the shared Save action, proves relative labels
  render without hover and without horizontal overflow, taps a relative label, and asserts the
  absolute stamp in the touch drawer. Restore the setting. The Last seen trigger and options get a
  mobile-appropriate touch target
  (min-h-11 / 44px at phone widths via responsive classes on this instance, per /mobile-parity —
  the shared Select defaults to 28px trigger/items), and the mobile E2E asserts the trigger's and
  option's bounding boxes meet 44px in the active dimension.

## Implementation waves

Wave 1:

- [x] [Task 01: Backend last_seen_display setting](task-01-backend-last-seen-setting.md)

Wave 2:

- [x] [Task 02: Frontend settings contract and SSR mapping](task-02-frontend-settings-contract.md)

Wave 3:

- [x] [Task 03: Security page option, relative column, tooltip, live update](task-03-security-page-relative-last-seen.md)

Wave 4:

- [x] [Task 04: E2E coverage (desktop + mobile)](task-04-e2e-relative-last-seen.md)

## Verification

```bash
make -C apps/backend test
cd apps/web && pnpm run typecheck && pnpm run lint:i18n components/settings/account/security-settings.tsx
cd apps/web && pnpm vitest run components/settings/account lib/ssr/user-settings.test.ts
cd apps/web && pnpm e2e:run --project auth tests/auth/relative-last-seen.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/auth/mobile-relative-last-seen.spec.ts
```

## Risks

- The select shares the floating Save and Reset actions with the other `/settings` configuration
  controls. Its table preview updates from the local draft, while the durable setting changes only
  after Save.
- Live ticking is wasted work if the user never switches to relative mode; the ticking interval is
  created only inside the relative-mode cell component, so absolute mode stays inert.
- Tooltip timing/positioning inside a table cell: use the standard Radix tooltip for fine-pointer
  devices. Coarse-pointer devices use the shipped Drawer pattern, and mobile E2E taps the label and
  asserts the visible absolute stamp.
- The hand-built backend JSON codec paths and the WS event snapshot are easy to miss because the
  model struct change alone looks sufficient; task 01 lists each path explicitly and tests the
  stored-JSON round trip and the published event map.
- The auth E2E project shares one worker backend whose SQLite DB survives restarts, and auth setup
  is single-shot; both new specs must use dedicated `KANDEV_DATABASE_PATH` values or the focused
  commands become order-dependent (the `users-self-actions` pattern).
- The PATCH write path is read-modify-write over a full settings blob; without expected-revision
  CAS, a concurrent omitted-field PATCH can revert `last_seen_display` at a newer revision that
  the frontend accepts. CAS + bounded retry is in task 01; the barrier test pins it.
