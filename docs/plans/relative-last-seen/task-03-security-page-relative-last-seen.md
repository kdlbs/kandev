---
id: "03-security-page-relative-last-seen"
title: "Security page option, relative column, tooltip, live update"
status: done
wave: 3
depends_on: ["02-frontend-settings-contract"]
plan: "plan.md"
spec: "../../specs/ui/requirements/relative-last-seen.md"
---

# Task 03: Security page option, relative column, tooltip, live update

## Acceptance

- The Active sessions card on `/settings/account/security` shows a labeled **Last seen** select
  (Absolute time / Relative time) above the table, with self-documenting copy and a discovery
  target. The card's `SettingsCard` keeps its existing `sessions` target (it accepts exactly one
  `discoveryTargetId`); the select's wrapper registers `ACCOUNT_SETTINGS_TARGETS.lastSeenDisplay`
  separately via `useSettingsTargetRegistration` from `settings-target-provider.tsx`, and a
  discovery-target test asserts both the sessions and the last-seen-display targets register and
  reveal. The trigger and options get a mobile-appropriate touch target (min-h-11 / 44px at
  phone widths via responsive classes on this instance; the shared Select defaults to 28px, below
  the /mobile-parity 44px active-dimension guidance).
- Default (`absolute`) renders `formatDateTime(last_seen_at)` exactly as before.
- Relative mode renders a locale-aware `formatRelativeTime(last_seen_at, now)` label inside a
  tooltip whose content is the absolute `formatDateTime` on fine-pointer devices. On
  coarse-pointer devices, tapping the label opens a Drawer with the same absolute timestamp. The
  fine-pointer trigger is focusable (`tabIndex={0}`, semantic element) and exposes the absolute
  timestamp as its accessible name and native `title` fallback. The label advances while the page
  stays open. An unparseable `last_seen_at` renders an empty cell with no disclosure surface
  (guards the `formatDateTime` `RangeError` on invalid dates).
- Changing the select keeps a local draft and marks the card dirty. Register the field with
  `useSettingsSaveContributor`. The shared Save action persists the draft through a queued
  `createQueuedUserSettingsSyncWithResponse({ last_seen_display })` write. The shared Reset action
  restores the confirmed store value. The store's `userSettings.lastSeenDisplay` is the confirmed
  baseline, updated by exactly two unchanged ingestion paths: (1) a successful PATCH response
  mapped with `mapUserSettingsResponse(response, current)` and applied THROUGH `setUserSettings`
  (never a raw store write; `setUserSettings` discards older revisions,
  `settings-slice.ts:245-250`); (2) a newer `user.settings.updated` WS snapshot through the
  existing handler path (`mapUserSettingsData` inside `store.setState` with the handler's own
  revision gate, `users.ts:12-24`), which Task 02 requires to stay UNCHANGED. On failure, keep the
  draft dirty and show error copy so the shared Save action can retry.
- While a Save request is in flight, a newer local draft stays visible. The backend event remains a
  full snapshot, and the unchanged WebSocket handler still applies own echoes and newer foreign
  values to the confirmed store. After a successful response, reconcile the draft with the
  revision-gated store result. A failed response keeps the draft dirty for retry. The component
  tests cover this behavior, PATCH-response ingestion through `setUserSettings`, WebSocket
  ingestion through the unchanged handler, Reset, and unmount/remount convergence.
- Component tests cover absolute default, relative label, tooltip content on hover, Tab focus,
  coarse-pointer Drawer disclosure, accessible name/title carrying the absolute timestamp, live
  ticking (fake timers), local draft state, shared Save and Reset actions, failed-save retry state,
  invalid timestamps, revision-gated PATCH responses, foreign WebSocket snapshots, and
  unmount/remount convergence. The tests exercise the PATCH-response ingestion path through
  `setUserSettings` and the WebSocket ingestion path through the unchanged handler.

## Verification

```bash
cd apps/web && pnpm run typecheck
cd apps/web && pnpm vitest run components/settings/account
cd apps/web && pnpm run lint:i18n components/settings/account/security-settings.tsx
```

## Files likely touched

- `apps/web/components/settings/account/security-settings.tsx` (+ new `security-settings.test.tsx`)
- `apps/web/lib/settings-discovery/catalog/account.ts`
- `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn,zh-hk,zh-tw}/account.json`

## Dependencies

Task 02 (store field and payload type must exist).

## Inputs

- Spec "What", "Failure modes", "Scenarios"
- Existing precedent: `ChangesPanelLayoutCard` select anatomy, `useNow(30_000)` live labels,
  `formatRelativeTime`/`formatDateTime` from `@/lib/i18n/formats`, Radix tooltip usage rules in
  `apps/web/AGENTS.md`

## Output contract

Return a compact handoff capsule with acceptance status, exact test command/results, risk tags,
uncertainties, and set this task to `done`.
