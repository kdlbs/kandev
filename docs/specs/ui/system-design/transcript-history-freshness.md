---
status: current
system: ui
requirements:
  - REQ-UI-TRANSCRIPT-HISTORY-FRESHNESS-001
created: 2026-10-07
owners:
  - kandev
---

# Transcript history freshness design

## Ownership and boundaries

This design extends the existing UI-owned
[bounded transcript projection](task-prompt-transcript-visibility.md).
It selects a complete revision for each matching ID in a latest-window read.
Tasks remains authoritative for persistence; Platform owns
[subscription recovery](../../platform/system-design/session-subscription-recovery.md).
Neither contract changes. There is no new event framework or API/schema work.

| Requirement | Design section |
| --- | --- |
| `REQ-UI-TRANSCRIPT-HISTORY-FRESHNESS-001` | Revision selection, Window membership, Runtime flow and scope, Verification boundary |

## Runtime flow and scope

`useSessionMessages` in `apps/web/hooks/domains/session/use-session-messages.ts`
waits for `getSessionSubscriptionReadiness` before issuing bounded WS
`message.list` (latest 100, descending). `requestSessionMessages` stores a shallow
array of immutable row references as `cachedAtRequest` on the shared request.
Followers use that original baseline, not their own later cache. After the
response, `fetchAndStoreMessagesAttempt` reads `cachedAtResponse`, invokes
`reconcileLatestMessageWindow`, checks the fetch sequence fence, and publishes
through the real `mergeMessages` action.

The latest path serves cached entry, reconnect/core recovery, foreground
refresh, running backfill, terminal fetch, and turn-settle resync. Core recovery
passes `force: true, authoritative: true`; the other paths use the same
reconciler without overriding membership. Preserve readiness, hydration keys,
sequence/generation guards, request sharing, loading, timeout, and retry logic.

Live `session.message.added/updated/deleted` frames enter
`apps/web/lib/ws/handlers/messages.ts`. The update scheduler coalesces by
session/message, checks freshness, and invokes actual `updateMessages`; add
invokes `addMessage`, delete flushes pending updates before removal. The store
also exposes `updateMessage`. Immer replaces changed row objects, while
`message-signature.ts` preserves identity for unchanged accepted snapshots.
Those references make the existing request baseline sufficient; no new live
event tracking is needed. A reference change proves a cache change, not its
transport origin: it may also represent another accepted read or local state.

`Message.updated_at` is optional in the frontend type. Backend
`internal/task/repository/sqlite/message.go` initializes it at creation and
`UpdateMessage` sets it to UTC now when persisting content/metadata. Public
live payloads preserve `updated_at` through `toMessage`. Wall-clock values are
revision hints, not a new monotonic server-version guarantee.

## Revision selection

In `message-window-reconciliation.ts`, resolve each fetched ID against both
cache maps before joining any extras. Compare update times with the existing
strict nanosecond parser `messageTimestampNanoseconds` from
`lib/state/slices/session/message-timestamp.ts`. Do not use lexical comparison,
millisecond rounding, `created_at`, or tool status to determine freshness.
Keep creation ordering at its existing backend microsecond precision.

| Cached/fetched revisions | Cache changed since request | Selected row |
| --- | --- | --- |
| Both valid, fetched strictly newer | Either | Fetched |
| Both valid, cached strictly newer | Either | Cached |
| Both valid, equal instant | Yes | Cached |
| Both valid, equal instant | No | Fetched, subject to existing identity reconciliation |
| Either missing or invalid | Yes | Cached |
| Cached valid, fetched missing/invalid | No | Cached |
| Cached missing/invalid, fetched valid or missing/invalid | No | Fetched |
| Fetched ID absent from current cache | N/A | Fetched |

A current row changed since issuance when its reference differs from the
baseline row for that ID; a missing baseline counts as an arrival. With no
comparable pair, even a valid fetched timestamp cannot prove it is newer than
an unversioned concurrent row. Retain that row conservatively until a later
read can hydrate without that race. Invalid includes malformed RFC3339 and
calendar dates that permissive `Date.parse` normalizes. Equivalent timezone
representations compare as the same instant; sub-millisecond differences remain
meaningful. Reuse `isIncomingMessageAtLeastAsFresh` for the unchanged-cache
fallback if useful; do not change its equal-time contract for other callers.

Select one entire row so content, raw result, status metadata, update time, and
other fields cannot become a fabricated mix. Preserve existing store carry
rules for prompt ordinals, retention markers, and resolved running notices.
No deep copy, mutation of the request baseline, or new global map is required.
Map lookups keep the correction linear in the window/cache size before the
existing chronological sort.

## Window membership

Compute fetched IDs, creation boundary, and overlap from the server response
and request baseline exactly as today. Then apply revision selection only to
IDs present in fetched rows, independently of the cache-extra filters:

- Overlap joins the selected fetched rows with existing older pages and live
  additions, deduplicates, and retains the actual oldest cursor.
- Disjoint replacement drops the old prefix. Cache additions not present at
  request time survive only at/after the fetched creation boundary. A fetched
  ID also added during the read must use revision selection, even though the
  live-addition filter excludes fetched IDs. The cursor remains the fetched
  boundary, not a discarded prefix or out-of-order older arrival.
- Authoritative replacement still removes absent request-baseline rows within
  the replacement interval, retains older pages, pending local rows, and new
  eligible cache rows under the current rules. Matching fetched IDs still need
  revision selection before joining retained extras.
- Non-authoritative empty responses preserve cache. Authoritative empty
  responses retain only pending local rows, even if other live changes arrived.

Freshness is not a deletion journal. A row absent from current cache but present
in the response remains subject to existing membership behavior. This repair
does not add a new tombstone guarantee or resurrect absent IDs as a retention
policy.

## Other read paths audited

`older-message-pagination.ts` uses HTTP `listTaskSessionMessages` with `before`
and calls `prependMessages`. That store action inserts only missing IDs and
keeps existing duplicates; it cannot overwrite a live result. Its request
sharing, first-request limit, purge guard, and older cursor remain unchanged.

`load-message-window.ts` uses HTTP `around` and `mergeWindowRows`, already
checking `isIncomingMessageAtLeastAsFresh` before accepting matching rows.
It has no request baseline and retains its current equal/missing-time behavior.
Boot hydration, plugin conversation projections, WS scheduler lexical guards,
and generic store reducers are not rerouted or redesigned by this change.

## Responsive and failure behavior

Desktop and phone chat composition use `use-chat-panel-state.ts`, hence the
same producer/store and tool renderer. The nearest surface remains
`components/task/task-layout.tsx`. This is state normalization inside an
unchanged surface: no responsive branch, navigation, geometry, scroll owner,
touch, or localized copy changes. The narrow state-only exception in
mobile-parity permits focused real hook/store/component coverage without a new
mobile Playwright scenario. If implementation changes presentation, revisit
that exception before proceeding.

Transport failures retain existing recovery behavior; this selector does not
introduce retries or loading ownership. Store/session lifecycle guards stay at
their existing boundaries. No persistence migration, permission change,
telemetry label, or reporter is needed.

## Verification boundary

Pure tests exercise all membership branches and the revision matrix. A new
rendered integration suite uses actual `useSessionMessages`,
`StateProvider/createAppStore`, actual store actions, and `ToolCallMessage`.
Only WS/HTTP transports are mocked. It holds history pending, changes a tool
through the real store, confirms the completed output is visible, settles an
older response, then measures the DOM outcome before any potentially failing
store assertion. Explicitly prove loading settlement and the selected row.
Controls accept newer server state, even after a live change, and preserve
distinct live additions. Cover an authoritative recovery callback as well as
normal refresh without bypassing the producer.

This is end-to-end coverage of the changed frontend boundary, not proof of a
running server, dispatcher, browser engine, or database. The original proof
measured pre-settlement rendering and post-settlement store regression only;
its following DOM assertion did not execute. Do not upgrade that claim.

## Related decisions and delivery

- [Separate task summaries from session streams](../../../decisions/2026-08-01-separate-task-summary-session-stream-traffic.md).
- [Replaceable session stream traffic](../../../decisions/2026-08-02-isolate-replaceable-session-stream-traffic.md).
- [Implementation plan](../../../plans/preserve-live-tool-results/plan.md).

The local selection rule reuses existing timestamps, immutable cache references,
and membership boundaries. Its rationale fits this design and its tests; it
does not establish a new architectural boundary requiring a separate ADR.
