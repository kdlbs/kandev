# SPA Failure Containment and Deployment Recovery

**Status:** accepted
**Date:** 2026-07-27
**Area:** frontend

## Context

Kandev serves a Vite SPA from the backend binary. The HTML shell is not cached,
while fingerprinted assets are immutable. A tab can therefore keep an older
entry bundle alive across a backend self-update and request a lazy route chunk
that the new binary no longer contains.

The SPA also has a single React root without a first-party application error
boundary. Settings and Office are lazy routes rendered with a null Suspense
fallback. An uncaught render or lazy-import error can empty the React root, and
a pending import can look like the same blank failure.

Two render-determinism defects compound this behavior:

- the Settings SPA adapter creates new thenables during render and passes them
  to client-side page components; and
- some Zustand selectors return new empty arrays for unchanged missing data,
  violating the stable-snapshot requirement of `useSyncExternalStore`.

These are cross-cutting runtime and deployment contracts rather than a new
product feature. ADRs 0012, 0021, and 0022 define self-update, the Go-served
SPA, and embedded Vite assets; this decision defines how the frontend remains
recoverable across failures at those boundaries.

## Decision

Once the SPA entry loads, every route transition must retain a visible,
recoverable surface.

### Failure containment

- A dependency-minimal root error boundary outside `StateProvider` catches
  provider, authentication, plugin-host, shell, and route failures. It renders
  a full-page alert with a native hard-reload action.
- A route error boundary inside `AppShell` contains route failures while
  preserving application chrome and navigation. It resets after an error only
  when the browser route changes.
- Narrower plugin error boundaries remain in place.
- Lazy Settings and Office routes use a visible, accessible loading surface.
  Loading, failure, and missing content must not be represented by an empty
  application root.
- User-facing recovery copy is generic. Raw errors and component stacks remain
  diagnostic output and are not displayed to users.

### Deployment-skew recovery

The frontend installs a `vite:preloadError` listener before React mounts.

- It stores `{ attemptedAt, href }` under `kandev.preloadRecovery` in
  `sessionStorage`.
- When no recent marker exists, it synchronously writes and verifies the
  marker, prevents Vite's later throw, and hard-reloads the document.
- A marker is recent for 60 seconds and permits at most one automatic reload in
  that tab during the window. A repeated failure is not prevented; it reaches
  the route or root boundary and presents recovery UI.
- If storage is unavailable or the marker cannot be verified, automatic reload
  is skipped so an unguarded cross-document loop is impossible.
- The marker is not cleared at initial mount. Clearing it before the lazy route
  succeeds could recreate the loop; expiration provides bounded recovery.

A rejected `React.lazy` import is not retried in-process because the rejected
module promise is cached. Recovery uses navigation or a hard page reload.

### Self-update handoff

Service self-update hard-reloads the document exactly once after
`/system/info` confirms the target backend version. The existing persisted
self-update marker is cleared before reload. Refreshing only update metadata is
insufficient because it leaves the old frontend module graph alive.

Desktop-native update behavior is unchanged.

### Render determinism

- SPA route adapters pass synchronous parsed identifiers to client components.
  They must not create promises or other thenables during render. Transitional
  server-style wrappers may remain only where a non-SPA caller still requires
  them.
- External-store selectors must return referentially stable empty collection
  snapshots. Module-level typed sentinels are the default for keyed data that
  has not hydrated.
- Office's existing cached parameter promises remain a transitional exception;
  converting that adapter is separate work.

## Consequences

- Route loading and failure remain distinguishable and visible on desktop and
  mobile.
- A stale lazy chunk normally self-recovers once; a persistent asset failure
  stops reloading and exposes an actionable fallback.
- Successful service updates replace the old HTML and JavaScript graph as soon
  as the new backend is authoritative.
- Settings route adapters and Zustand selectors have explicit identity
  contracts that are enforceable with focused tests.
- Error boundaries do not catch event-handler or arbitrary asynchronous
  failures. Those paths still require local error handling.
- `sessionStorage`-blocked environments skip automatic preload recovery but
  still receive boundary-based recovery UI.

## Alternatives Considered

### Only add a root error boundary

Rejected. It prevents a permanently empty root but discards usable shell
navigation for route-local failures and does not distinguish loading from
failure.

### Retry rejected lazy imports in the current document

Rejected. `React.lazy` caches the rejected import promise, so an in-process
retry is not reliable.

### Reload on every preload error

Rejected. A missing or corrupt asset could create an infinite cross-document
reload loop.

### Retain multiple generations of embedded assets

Deferred. It increases packaging and asset-lifecycle complexity and does not
address render errors, unstable selectors, or async client components.

### Cache Settings parameter promises

Rejected as the primary fix. Stable promises can avoid identity churn for
`use(params)` consumers but do not make async client component wrappers valid.
Synchronous SPA props remove both suspension sources.

### Redesign Settings bootstrap and all null page states

Deferred. Cancellation, stale-data retention, and page-specific loading states
deserve separate work. They are not required to contain the diagnosed root,
lazy-chunk, thenable, and selector failures.
