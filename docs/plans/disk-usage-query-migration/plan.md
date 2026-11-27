---
created: 2026-10-05
updated: 2026-10-07
status: in_progress
requirements:
  - REQ-SYSTEM-PAGE-SYSTEM-PAGE-001
system_design:
  - ../../specs/system-page/system-design/disk-usage-query-cache.md
legacy_specs: []
---

# Implementation Plan: Disk Usage Query Cache Migration

## Overview

Move the System Status disk-usage snapshot and GET request state from Zustand
and local React state into the existing stable TanStack Query client. Retain
existing system-job stream in Zustand and add one resource-specific observer
that invalidates the exact disk query after a terminal disk-walk event. The
server API, refresh action, conditional 1.5-second recovery, and card layout
remain unchanged. Implementation starts after the QUERY-02 database migration
and QUERY-03 backup migration have merged into `main`, and the shared identity
provider contract is re-read from that base.

## Scope

### In scope

- Give the disk response and GET loading, error, and request lifecycle one
   owner in TanStack Query; keep only separate refresh POST feedback local.
- Remove only `diskUsage` state, its action, and its default from the System
  slice; keep jobs and every other System resource in Zustand.
- Revalidate the current resource key on terminal disk-walk success and
  failure, with bounded identity-scoped event deduplication.
- Keep explicit refresh behavior and conditional polling while the most
  recent snapshot says `computing=true`.
- Preserve current desktop and phone presentation and the shared card
  interaction.

### Out of scope

- Changes to backend disk-walk service, two-hour cache, job tracker, WS wire
  protocol, reconnect policy, or control-plane probes.
- Migration of the job stream or any System resource other than disk usage.
- A global event bus, cache-wide reset, new product behavior, or visual
  redesign.
- Unconditional refetch on focus or reconnect.

## Technical approach

1. After QUERY-03 merges, re-read the shared System Query identity helper,
   stable provider, cleanup behavior, and test contract. Use the same full
   normalized backend URL, page boot ID, auth mode/status, and user ID. Do not
   add workspace to the key or copy an identity normalizer.
2. Add the disk snapshot to Query with explicit freshness and retry options in
   [the design](../../specs/system-page/system-design/disk-usage-query-cache.md).
   Pass Query's native `AbortSignal` through `fetchDiskUsage` to
   `fetchJson`. Keep the authenticated shell and QueryClient stable; cancel
   and immediately remove only obsolete disk-usage keys.
3. Add one authenticated-app disk job bridge. It observes the existing
   `system.jobs` map, baselines retained jobs on mount and identity change,
   filters terminal `disk-walk` rows, and invalidates only the current key.
   Use a FIFO capped at 64 terminal IDs per identity plus the existing row's
   terminal transition to suppress duplicate updates after FIFO eviction.
   Leave `registerSystemEventsHandlers` and all job consumers as stream owners.
   Use a synchronous subscription to the owning store so batched terminal and
   running replays cannot hide an accepted completion. Keep no second job map.
   A provider without disk consumers must remain lazy. An existing inactive
   query retains invalidation until the next consumer mounts.
4. Replace hook-local Zustand snapshot and GET loading/error state with Query
   data and request state. Keep refresh as POST followed by GET, with only
   distinct POST feedback local; both public hook actions continue to resolve
   after handled failures. Keep `isLoading` true during initial and refetch
   GETs, preserve last-good data on GET errors, and retain the 1500ms interval
   only while current query data says `computing=true`. Polling remains
   periodic without an attempt cap; it does not discover a whole unseen job
   when cached data is `computing=false`.
   Capture the POST's key and hook lifecycle generation. After POST settles,
   verify both before GET or feedback updates, including A→B→A and remounts.
   Follow the design's error precedence and reset rules for the one card row.
5. Before finalizing event invalidation, run deferred-response tests against
   installed TanStack Query for an in-flight GET with no cached data and for a
   cached-data refetch. Deliver completion, resolve the older GET with stale
   `computing=false` and `computing=true` after its signal is aborted, and
   prove a post-event authoritative response wins. Combine delayed cancel
   completion with A→B→A; a stale callback must not invalidate/refetch/remove
   the new A entry or duplicate reads. Guard the captured full identity and
   bridge baseline/subscription lifecycle, and operate on captured exact keys.
   Measure request counts during duplicate/out-of-order terminal bursts. Start
   with exact-key cancellation then invalidation; keep that mechanism only if
   the installed APIs prove these schedules. Do not add a custom request
   coordinator without evidence that the library cannot meet the acceptance.

### Compatibility and lifecycle

| Shape | Intended behavior | Evidence or boundary |
| --- | --- | --- |
| Auth enabled, authenticated user | Shared backend/boot/auth key, stable shell, obsolete disk keys removed | Hook/provider identity tests |
| Auth disabled | Existing default-user identity from the shared helper | Hook/provider auth-disabled regression |
| Logout or last disk observer leaves | Native GET cancellation, no stale feedback, subscriptions cleaned up | Deferred hook and bridge tests |
| App mounted without disk consumers | No eager GET or cache entry creation. Existing inactive entries retain invalidation | Bridge tests and route-return hook test |
| StrictMode effect replay or multiple consumers | Shared GET ownership, no leaked bridge subscription, eventual authoritative recovery | Deferred hook/provider/bridge tests |
| Page-static backend URL | Existing WS source and Query key remain aligned | Current connector and identity source inventory |
| Runtime-mutable backend URL or provenance-free retained job replay | No expanded transport guarantee | Separate event-source review required for mutable URLs. FIFO/store-removal replay limit remains documented |

## Tests

| Scenario | Evidence |
| --- | --- |
| Existing disk response, refresh POST, handled-error resolution, POST error display, last-good snapshot, and calculating state | `hooks/domains/system/use-disk-usage.test.tsx` and desktop/mobile disk-usage E2E |
| Actual `system.job.update` handler → Zustand job map → disk bridge → Query invalidation; success and failure | `components/system-disk-usage-query-bridge.test.tsx` using `registerWsHandlers` |
| Duplicate, out-of-order running-then-terminal, retained initial/hydrated, FIFO-evicted terminal IDs, and unrelated job burst | Bridge integration tests with deterministic store and QueryClient state |
| After FIFO eviction, row removal or terminal→running→terminal replay can cause another read | Bridge tests documenting the bounded-history limit without changing WS/store ownership |
| Terminal then running replay in one React batch; later deep-merged job additions; StrictMode subscription replay and cleanup | Bridge tests using real synchronous store updates, not mocked invalidation alone |
| Event during first no-data GET and cached-data refetch; stale response after abort with `computing=false` or `true`; delayed cancel completion during A→B→A; bounded requests under bursts | `hooks/domains/system/use-disk-usage.test.tsx` and bridge tests using installed Query APIs, deferred transport responses, and fake timers |
| Backend/auth/boot identity changes, A→B→A, logout/unmount cancellation, obsolete-key removal, stale GET/POST feedback isolation, and unchanged shell state | `components/system-disk-usage-query-bridge.test.tsx` and `hooks/domains/system/use-disk-usage.test.tsx` |
| Late successful POST after identity change or hook remount does not start GET; POST failure starts no follow-up GET; POST→GET error precedence and reset on recovery | Deferred hook tests and desktop refresh-error E2E |
| No consumer means no eager query; inactive terminal invalidation and departure between cancel/invalidate preserve data and refetch on return | Bridge and hook tests with real QueryClient cache inspection |
| Initial missed event, refresh followed by `computing=true`, interval recovery, stop at `computing=false`, and repeated interval errors | Hook tests with deterministic timers |
| Desktop refresh/display and unchanged responsive card | Existing `e2e/tests/system/disk-usage.spec.ts` and focused `e2e/tests/system/mobile-disk-usage.spec.ts` |
| Mobile System Status remains usable with the same card and refresh controls | New `e2e/tests/system/mobile-disk-usage.spec.ts` in the `mobile-chrome` project |
| Requirement coverage | Tests exercising visible AC-SYSTEM-PAGE-SYSTEM-PAGE-001.2 outcomes reference that AC where useful |

## E2E tests

The same shared `DiskUsageCard` remains at desktop and phone widths, with no
viewport-dependent layout or behavior change. The desktop chromium flow passed
5/5 and the focused mobile-chrome flow passed 1/1 at 390×844. Both assert
refresh POST then GET plus visible breakdown and timestamp; the mobile test
also checks horizontal overflow. Conditional missed-event recovery is covered
with deterministic hook tests. The existing first-load browser test does not
force a cold cache or a dropped WS event.

## Work orders

- [ ] [Task 01: Move disk usage to Query with job-event recovery](task-01-disk-usage-query.md)

## Verification results

Dependencies and implementation evidence:

- QUERY-02 merged as `059260b30fc68bbcbead629f7fa7e80f1ee0a5e8`; QUERY-03
  merged as `d149627883ca74de0dde98c1900415729af44bbd`. Both are ancestors of
  dependency-prepared base `b3f207b2e7f08f39c628db8b1d5d0a1374e1b3d7`.
- TanStack Query 5.104.0 deferred-response tests prove exact-key cancellation
  and invalidation replace a no-data initial GET and a cached-data refetch.
  Already-aborted stale responses with either value of `computing` do not
  overtake the authoritative post-event read. Delayed cancellation across
  A→B→A does not affect the new identity.
- The bridge burst with 16 distinct terminal job IDs makes 17 total GETs:
  one initial read plus one replacement per accepted event. Duplicate and
  out-of-order replays add zero reads. Tests enforce the 64-ID FIFO and cover
  the documented eviction limit when terminal store evidence is removed or
  changed to nonterminal.
- Focused web tests: 12 suites and 145 tests passed. Typecheck and full web
  ESLint passed after review fixes. Desktop disk-usage E2E passed 5/5; mobile
  Status-card E2E passed 1/1 at 390×844. Both verify refresh POST then GET and
  visible data; mobile arms the initial GET wait before navigation.
- The PR frontend check exposed an incomplete Zustand store mock in
  `app-error-boundary.test.tsx`: the disk bridge reads `system.jobs` and
  subscribes to job changes. The original test failed on the resulting root
  error screen. The mock now supplies an empty jobs map and unsubscribe
  function. With `NODE_ENV=production`, the full file passed 8/8; targeted
  ESLint and full web typecheck passed. This is test-fixture maintenance and
  does not change product behavior.
- The required E2E run exposed a 30-second response assertion in the existing
  managed-clone recovery test, before the session reached `WAITING_FOR_INPUT`.
  The test now waits up to 120 seconds for the established resumed-session
  state before checking the successful response. The focused scenario passed
  5/5 on a synthetic current-main merge under the CI runtime image capped at
  2 CPUs and 4 GiB. This fixup changes no product behavior.
- Architecture lint, docs catalog validation, specification lint and its 36
  tests, PR documentation coverage (`covered`, no errors), and `git diff
  --check` passed.

## Delivery

PR [#4291](https://github.com/kdlbs/kandev/pull/4291) is open against
`main`. Its initial implementation commit is
`7f5319f3f191c9f8310314cf2e3201fa0fbc6f48`; its review-fixup commit is
`f19509441558d3354f09774b496246a694854e9a`. PR creation observed base
`1204f0e5488418d0d9aa9aaaf6a31988ccdfcd8f`. During fixup, `main` advanced to
`330e02a47808c11ca315ae30456fcce7f4806db5`; the only overlapping file,
`apps/web/lib/state/app-state-types.ts`, adds the separate workspace-recovery
projection. A synthetic merge at that base was conflict-free; 145 focused tests
and `pnpm run typecheck` passed on the merged tree, so no rebase was needed.
Required checks and AI reviews continue on the pushed fixup. Keep this task in
progress until exact-head checks are terminal and all review threads are
dispositioned. Do not merge. Hand off final head/base SHAs, owned files, command
results, request counts, exact-head CI/review status, residual risks, and
merge-order notes to the parent.

## Risks

- Deferred-response tests against installed TanStack Query 5.104.0 prove the
  selected cancel-then-invalidate behavior for both no-data and cached GETs.
- `WebSocketConnector` captures its page backend URL once and reconnect does
  not replay system jobs. A runtime-mutable backend URL is outside the current
  contract and requires a separate event-source review.
- Polling can continue at 1.5-second intervals after repeated errors while
  retained data still says `computing=true`; this matches the current hook and
  is not a bounded retry count.
- The terminal store-row guard depends on the row retaining terminal state.
  After FIFO eviction, a nonterminal replay or row removal can erase that
  evidence, and a later terminal replay can revalidate. The 64-ID bound is not
  an unlimited at-most-once guarantee.

## Resolved implementation question

Exact-key `cancelQueries` followed by `invalidateQueries` in TanStack Query
5.104.0 replaces both no-data and cached in-flight GETs. Deferred tests resolve
already-aborted old transports with stale `computing=false` and `true` data and
confirm the authoritative post-event response wins without a coordinator.
