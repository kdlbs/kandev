---
id: "02-fix-background-refresh-loading-state"
title: "Fix Needs-you Inbox background-refresh loading state"
status: done
wave: 2
depends_on:
  - "01-needs-you-inbox-delivery"
plan: "plan.md"
requirements:
  - REQ-UI-NEEDS-YOU-INBOX-001
acceptance_criteria:
  - AC-UI-NEEDS-YOU-INBOX-001.19
  - AC-UI-NEEDS-YOU-INBOX-001.20
  - AC-UI-NEEDS-YOU-INBOX-001.21
system_design:
  - ../../specs/ui/system-design/needs-you-inbox-02.md
---

# Task 02: Fix background-refresh loading state

## Summary

`resolveViewMode` (`apps/web/app/needs-you-inbox/needs-you-inbox-page-client.tsx`)
implemented AC .20/.21 by branching on `status` alone, which cannot tell a
refresh that just succeeded from one that just failed: both leave `status`
momentarily at `"loading"` while the request that will resolve it is in
flight. The observable defect was the Inbox re-showing "Loading..." on every
background refresh trigger from
[Control flow](../../specs/ui/system-design/needs-you-inbox-02.md#control-flow)
(WS event, reconnect, tab-visibility regain, or the 60-second periodic
re-read) even when the list was already settled and empty, and — with no
active workspace resolved — never leaving the loading view at all, because no
read is ever issued to resolve it.

Separately, the WS refresh trigger's coalescing window was leading-edge only:
an `ws.ActionSessionPendingActionChanged` / `ws.ActionSessionStateChanged`
event that landed inside the 250 ms window was dropped rather than queued,
so the re-read AC .19 requires ("no exit shall be able to strand a row by
emitting no event... the row set shall converge from re-reading the bundle
path") did not always run.

## In scope

- `needsYouInbox` slice: add `lastAppliedOk` (true after a successful list
  apply, false after an error, boot-seed, or default), surviving the
  `status -> "loading"` overwrite a refresh performs.
- `resolveViewMode(status, bundleCount, hasMore, lastAppliedOk,
hasActiveWorkspace)`: gate the loading view on `!lastAppliedOk` instead of
  `status` alone, and short-circuit to the empty view when
  `hasActiveWorkspace === false` (previously: no trigger ever re-resolves the
  view, so the loading state was permanent).
- `use-needs-you-inbox-controller.ts` (`useNeedsYouInboxWsRefresh`): keep the
  leading-edge read but queue one trailing read for a trigger that lands
  inside the coalescing window, instead of dropping it.
- E2E: `apps/web/e2e/tests/chat/needs-you-inbox-background-refresh.spec.ts`.

## Out of scope

Any change to the read contract, the row/count derivation, or the five
refresh triggers themselves (still exactly the ones
[Control flow](../../specs/ui/system-design/needs-you-inbox-02.md#control-flow)
names). This task fixes how the client renders the interval between a
trigger firing and its response landing; it defines no new trigger and reads
no new field.

## Acceptance

- AC .19: a WS refresh trigger landing inside the coalescing window is queued
  and still runs, rather than being silently dropped — regression test in
  `use-needs-you-inbox-controller.test.tsx`.
- AC .20: with `hasActiveWorkspace === false`, the Inbox renders the empty
  view rather than an unresolvable loading view.
- AC .21: `resolveViewMode` renders the error view (not the loading or empty
  view) for every reachable `(status, bundleCount, hasMore, lastAppliedOk,
hasActiveWorkspace)` tuple in which the most recent apply was an error.

## Verification

- `npx vitest run` (3 touched files): 36/36 passed.
- `pnpm run typecheck`: clean.
- `pnpm e2e:run --project chromium -- tests/chat/needs-you-inbox-background-refresh.spec.ts`:
  passed.
- Manual dev-instance check (flag forced on): empty state settles with no
  loading flash on first load, reload, or background-refresh tick.
- Own analysis enumerated all 12 reachable `(status, bundleCount, hasMore,
lastAppliedOk, hasActiveWorkspace)` tuples against `resolveViewMode`, all
  correct; independently cross-checked by a second model with no production
  defect found.

PR: https://github.com/kdlbs/kandev/pull/3685
