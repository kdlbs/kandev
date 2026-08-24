---
status: active
system: ui
created: 2026-08-16
owners:
  - Kandev
---
# Reload Kandev when a tab is restored from a frozen browser snapshot Requirements

## Overview

Duplicating a Kandev tab in Chrome restores a frozen snapshot of the page (back/forward-style restore, not a fresh load; Chromium reports the duplicated tab's navigation type as `back_forward`). The restored page keeps the JS heap and DOM as they were when the snapshot was taken, and no boot payload is re-fetched.

## Requirements

### REQ-UI-FIX-DUPLICATED-TAB-STALE-DATA-001: Reload Kandev when a tab is restored from a frozen browser snapshot

**Intent:** Duplicating a Kandev tab in Chrome restores a frozen snapshot of the page (back/forward-style restore, not a fresh load; Chromium reports the duplicated tab's navigation type as `back_forward`). The restored page keeps the JS heap and DOM as they were when the snapshot was taken, and no boot payload is re-fetched.

#### Acceptance criteria

- **AC-UI-FIX-DUPLICATED-TAB-STALE-DATA-001.1:** When the Kandev page is restored from a frozen browser snapshot, the app performs a full reload so state is re-fetched from the backend.
- **AC-UI-FIX-DUPLICATED-TAB-STALE-DATA-001.2:** A restore is detected on the `pageshow` event when `event.persisted === true`. This is the only reliable frozen-restore signal: the navigation type `back_forward` also covers cold history traversals and session-restored tabs, which load fresh and must not be reloaded a second time.
- **AC-UI-FIX-DUPLICATED-TAB-STALE-DATA-001.3:** Normal page loads are unaffected: a fresh navigation, a manual refresh, a cold back/forward traversal, and in-app SPA routing never trigger the reload.
- **AC-UI-FIX-DUPLICATED-TAB-STALE-DATA-001.4:** **GIVEN** a Kandev tab showing a task in Active tasks, **WHEN** the task is archived and the user duplicates the tab in Chrome, **THEN** the duplicated tab reloads and the task is not shown as active. *(Pending: the native Duplicate-tab event sequence is the outstanding verification gate; until it is recorded, this outcome is expected but not yet evidenced.)*
- **AC-UI-FIX-DUPLICATED-TAB-STALE-DATA-001.5:** **GIVEN** a loaded Kandev page, **WHEN** the browser restores it from bfcache (back/forward navigation), **THEN** the page reloads with fresh data.
- **AC-UI-FIX-DUPLICATED-TAB-STALE-DATA-001.6:** **GIVEN** a Kandev page loading normally, **WHEN** it loads, **THEN** no reload is triggered.
- **AC-UI-FIX-DUPLICATED-TAB-STALE-DATA-001.7:** **GIVEN** a Kandev page, **WHEN** the user refreshes it manually, **THEN** no additional reload is triggered.
- **AC-UI-FIX-DUPLICATED-TAB-STALE-DATA-001.8:** **GIVEN** a Kandev page reached by a cold back/forward traversal (loaded fresh, `pageshow.persisted` false), **WHEN** it loads, **THEN** no reload is triggered.

## Migrated source detail

## Why

Duplicating a Kandev tab in Chrome restores a frozen snapshot of the page
(back/forward-style restore, not a fresh load; Chromium reports the duplicated
tab's navigation type as `back_forward`). The restored page keeps the JS heap
and DOM as they were when the snapshot was taken, and no boot payload is
re-fetched.

Kandev's data-freshness model assumes fresh loads: the SPA shell and all data
fetches are `Cache-Control: no-store`, so any real navigation re-reads current
backend state. A restored page bypasses that model entirely. The existing
foreground-refresh hooks (`useForegroundRefresh`) are best-effort, cover only
subsets of surfaces, and do not distinguish restores from normal focus events,
so a frozen restore can show stale data (for example, a task archived after
the snapshot still appears in Active tasks) until the user manually refreshes.

Chrome has been rolling out back/forward cache (bfcache) admission for
`Cache-Control: no-store` pages since 2024, so restores of this page are
becoming more common. The platform guidance for this exact situation is to
handle `pageshow` with `event.persisted === true` and refresh or reload the
page.

## What

- When the Kandev page is restored from a frozen browser snapshot, the app
  performs a full reload so state is re-fetched from the backend.
- A restore is detected on the `pageshow` event when `event.persisted ===
  true`. This is the only reliable frozen-restore signal: the navigation type
  `back_forward` also covers cold history traversals and session-restored
  tabs, which load fresh and must not be reloaded a second time.
- Normal page loads are unaffected: a fresh navigation, a manual refresh,
  a cold back/forward traversal, and in-app SPA routing never trigger the
  reload.

## API surface

No backend, network, or public contract change. Observable behavior: after
Chrome's Duplicate tab (or a back/forward restore) the page reloads once
instead of showing frozen state. This is the same effect as the user's manual
refresh, automated.

## Failure modes

- Restore signals unavailable (no `PageTransitionEvent.persisted`): the
  handler degrades to a no-op and stale data can still appear until a manual
  refresh. All browsers that implement bfcache deliver `persisted` on the
  restore.
- Cold history traversals and session-restored tabs fire `pageshow` with
  `persisted` false even though their navigation type is `back_forward`; the
  handler ignores them because those pages load fresh and need no reload.
- Reload loop: a reload produces a fresh document whose `pageshow.persisted`
  is `false`, so no recursive reload.
- Open WebSocket at freeze time can make the page ineligible for bfcache on
  back/forward navigations in some Chrome versions; such navigations reload
  normally and are correct without the handler. Genuine restores deliver
  `persisted` on `pageshow`. Whether Chrome's native Duplicate-tab path is
  such a restore (vs. a state clone that fires no `pageshow` at all) is
  pending the native verification gate documented in the plan; the
  Duplicate-tab outcome in the scenarios below is marked pending until that
  evidence is recorded.

## Persistence guarantees

Server state remains the source of truth; the reload re-reads it. The fix
writes no server state.

Client storage: in debug builds only (`window.__KANDEV_DEBUG`), a
`pageshow` duplicate/history candidate writes a diagnostic record to
sessionStorage under `kandev.bfcacheRestoreProbe` before any reload decision —
`{ persisted, navigationType, at }`. Candidates are frozen restores
(`persisted === true`, which then reload) and `persisted === false` documents
reached via a `back_forward` traversal (diagnostic only, no reload), so a
native Duplicate-tab run can be verified after the reload destroys the
document. The write is best-effort (never blocks the reload), is skipped
outside debug builds, and is scoped to the tab session: it is overwritten on
the next candidate and cleared by the merge-gate checklist before each
attempt. It contains no user data.

## Scenarios

- **GIVEN** a Kandev tab showing a task in Active tasks, **WHEN** the task is
  archived and the user duplicates the tab in Chrome, **THEN** the duplicated
  tab reloads and the task is not shown as active. *(Pending: the native
  Duplicate-tab event sequence is the outstanding verification gate; until it
  is recorded, this outcome is expected but not yet evidenced.)*
- **GIVEN** a loaded Kandev page, **WHEN** the browser restores it from
  bfcache (back/forward navigation), **THEN** the page reloads with fresh
  data.
- **GIVEN** a Kandev page loading normally, **WHEN** it loads, **THEN** no
  reload is triggered.
- **GIVEN** a Kandev page, **WHEN** the user refreshes it manually, **THEN**
  no additional reload is triggered.
- **GIVEN** a Kandev page reached by a cold back/forward traversal (loaded
  fresh, `pageshow.persisted` false), **WHEN** it loads, **THEN** no reload
  is triggered.

## Out of scope

- Incremental state reconciliation after restore. The reload is the
  reconciliation; WS reconnect/resubscribe continues to cover session-level
  data independently.
- Reloading on tab freeze/resume (`resume` event). Backgrounded-tab resume is
  the normal tab-switch flow already handled by the existing
  foreground-refresh hooks.
- Changing HTTP caching headers. The shell and data fetches already use
  `no-store`; bfcache is not the HTTP cache and cannot be disabled with
  headers.
