---
status: current
system: ui
requirements:
  - REQ-UI-SESSION-SEARCH-OWNERSHIP-001
---

# Session Search Ownership System Design

## Purpose and boundaries

Keep request admission and settlement inside the lifetime of the existing
`useSessionSearch` instance. This extends its local cancellation boundary; it
does not introduce a request coordinator, store, shared framework, or backend
contract. UI owns client reply ownership independently of message persistence.
No meaningful architectural alternative or new ADR is required for this local
correction. Existing specification statuses and migration history remain intact.

## Requirement mapping

| Acceptance | Design boundary |
| --- | --- |
| `AC-UI-SESSION-SEARCH-OWNERSHIP-001.1` | Immediate query retirement and settlement checks |
| `AC-UI-SESSION-SEARCH-OWNERSHIP-001.2` | Close, reopen, timer admission |
| `AC-UI-SESSION-SEARCH-OWNERSHIP-001.3` | Committed session and mounted lifetime |
| `AC-UI-SESSION-SEARCH-OWNERSHIP-001.4` | Current request, transport, navigation controls |

## Source, consumer, and transport inventory

| Existing file and symbols | Responsibility and disposition |
| --- | --- |
| `apps/web/hooks/domains/session/use-session-search.ts`: `useSessionSearch`, `useDebouncedSearch`, `useSearchContext`, `useSetActiveHit`, `focusMessageElement` | Sole production correction boundary; retain public `SessionSearchHook` shape, debounce and MAX40 navigation controls |
| `apps/web/components/task/task-chat-panel.tsx`: `TaskChatPanel`, `SessionSearchOverlay`, `resolvedSessionId`, `loadMoreRaw`, `navigateSearchHit` | Actual production hook consumer; selected session and raw-backfill/scroll-owner callbacks feed the hook |
| `apps/web/components/search/panel-search-bar.tsx`: `PanelSearchBar`, `useDebouncedChange` | Overlay routes onChange to setQuery and onClose to close; debounceMs=0 still queues a timer; audit retained callbacks, retain shared component behavior |
| `apps/web/components/task/chat/session-search-hits.tsx`: `SessionSearchHits` | Renders query, hits, selection and loading supplied by the overlay |
| `apps/web/hooks/use-panel-search.ts`: `usePanelSearch` | Existing open/close keyboard entry point; actual path and callback wiring verified |
| `apps/web/lib/api/domains/session-api.ts`: `searchSessionMessages`, `MessageSearchHit`, `SearchMessagesResponse` | Real API maps selected session/trimmed query/limit50 into message.search; retain no-client empty response and 10000ms request timeout |
| `apps/web/lib/ws/connection.ts`: `getWebSocketClient`, `setWebSocketClient` | Existing client connection seam; production unchanged |
| `apps/web/lib/ws/client.ts`: `WebSocketClient.request`, `handleRequestResult`, pending-request resolution/rejection | Real request IDs and protocol settlement; production unchanged |
| `apps/web/hooks/domains/session/use-session-search.test.ts` | Existing API-mocked debounce/newer-request/null/close/navigation controls, kept alongside real-protocol regression |
| `apps/web/hooks/domains/session/use-session-search-ownership.test.tsx` | Rendered production hook with only deferred WebSocket wire transport substituted |

The only production consumer found by the source inventory is TaskChatPanel.
SessionSearchOverlay forwards the actual hook result rather than deriving another
ownership predicate. No immediate glue change is planned; require causal evidence
and a scope checkpoint before expanding this production boundary.

## Local ownership and control flow

Use local refs for mounted/committed-session lifetime and request generation.
Capture ownership when scheduling a query, not only when a request begins.
Retire that generation synchronously at each accepted query edit and close.
Clear obsolete hits and active selection immediately; clear loading until the
new eligible request begins. Empty or whitespace-only input performs no request.

On committed session change, including null/undefined, retire queued and pending
work and clear query/hits/selection/loading without automatically searching the
former text in the new session. Preserve the user's open/closed overlay choice.
Unmount cleanup retires ownership and clears owned timers. Commit-bound setup and
cleanup must remain usable through React StrictMode effect replay; never mutate
ownership for an abandoned render or leave the current instance permanently dead.

The delayed callback verifies mounted lifetime, committed session identity, query
generation, and search eligibility before starting the API call. Retained public
callbacks from a previous committed session must also fail admission, including
an A-to-B-to-A transition. Current callbacks remain usable across ordinary
rerenders and query edits. Close retires the open search lifetime, so its queued
query callbacks cannot admit work after reopen either; freshly rendered callbacks
must support new searches after reopen. Keep current open/close and navigation
callbacks usable according to their existing public contracts.

At success, catch, and finally, verify the same ownership plus newest request
identity before touching state or logging a failure. Retired success does nothing;
retired failure does not clear newer hits or log a misleading current error;
retired finalization cannot clear a newer request's loading indicator. Current
success uses `resp.hits ?? []`; current failure retains the existing console
diagnostic, empty hits, and settled loading. No transport abort or retry is added.

Keep `useSetActiveHit` cancellation, transcript-owned navigation and
`MAX_BACKFILL_ITERATIONS=40` independent of search request ownership. Do not
redesign loaded message identity or raw-backfill policy. All counters/refs are
instance-local so one panel cannot invalidate another.

## Regression seam and coverage

Render the actual production hook using React Testing Library, import the real
session API and WebSocketClient, and install an owned deferred wire transport
through the existing connection seam. Reply or reject using actual outbound
request IDs. Assert exact payload, state and absence of unwanted outbound requests;
do not mock the API, ownership predicate, protocol parser, or duplicate the hook.
Restore client/global state and join owned cleanup for each fixture. Do not export
private production helpers or recreate TaskChatPanel in the test.

Cover query replacement before 180ms and after a newer request begins; clearing
including whitespace; close/reopen before old success or failure; old finalizer
while the new request is pending; queued work cancelled on close/session change;
session A-to-B and A-to-null-to-A; retained callbacks; unmount; StrictMode replay;
two independent instances; current results/errors and exact wire identity. Extend
only focused existing navigation/backfill controls if their preservation needs
coverage. The accepted ROOT proof supplies the historical causal failing cases;
permanent tests were written and causally failed before the hook correction after
ROOT's later implementation release. The 22 real-protocol tests and seven existing
search tests pass with one worker and no file parallelism.

## Responsive and documentation audit

The correction is purely state/data normalization in the existing shared hook.
No layout, touch, scrolling, navigation, breakpoint, or rendered-copy change is
planned. `/mobile-parity` explicitly allows targeted unit/component tests and this
note in that case; no browser, E2E, build, or ASCII layout preview is needed.
The shared TaskChatPanel semantics apply to desktop and phone.

Public docs guidance, `docs/public/**`, root README and screenshot catalog were
audited. No affected search-lifetime instructions were found; APIs, labels,
navigation and screenshots do not change. This draft package records internal
implementation contract. No public page or coverage-map change is needed; the
existing public-doc validator passed after release. Checkpoint any newly demonstrated
authoritative documentation gap rather than adding uncontrolled artifacts.
