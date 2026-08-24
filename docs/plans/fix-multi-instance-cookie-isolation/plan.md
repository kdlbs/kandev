---
spec: docs/specs/auth/requirements/fix-multi-instance-cookie-isolation.md
created: 2026-08-17
status: pending
---

# Implementation Plan: Isolate Kandev Cookies Between Instances on One Host

## Root cause (confirmed)

Cookies are scoped by scheme + host only; the browser ignores port when
deciding which cookies to send. Multiple kandev instances on one IP (different
ports) therefore share one cookie jar, and kandev stores per-instance identity
in host-scoped cookies:

- `kandev_session` (auth token): last login wins; every other auth-enabled
  instance rejects the foreign token (per-instance session store) and 401s,
  and the SPA's `onUnauthorized` handler clears auth state and redirects to
  `/login`. Reproduced live: two auth-enabled instances on :48429/:48529 each
  return `{"authenticated":false}` for the other's token, and logging into
  one yanks the other's open UI to the Sign in page. Maintainer-documented in
  #2689.
- `kandev-active-workspace` / `office-active-workspace`: foreign workspace ids
  are read at boot by every other instance. Both sides validate and fall
  back today, so this path is graceful, but it churns cross-instance
  workspace/mode selection and is the same unsegregated-storage class.

Fix: port-scope the cookie **names** (suffix `_<port>`), derived from the
API origin port on both sides — request Host / `X-Forwarded-Host` (backend)
and `getBackendConfig().apiBaseUrl` (frontend; `window.location.port` only
equals it in same-origin deployments). Legacy unprefixed workspace cookies
stay readable (validated fallback); the session cookie has no fallback (one
re-login, bug stays dead during migration).

## Changes

### Backend

- **New package `apps/backend/internal/common/httpcookie`**: `PortSuffix(r)`
  and `ScopedName(r, base)`; honors `X-Forwarded-Host` (first value) before
  `Host`; no suffix when the host has no port; IPv6 bracketed hosts handled.
  Unit tests. Shared by the auth service and the boot-state builder.
- **Session cookie scoping** (`apps/backend/internal/auth`): login/setup/
  logout handlers (`httpapi/cookie.go`, `handlers.go`), invite accept
  (`handlers_admin.go` — there is no separate admin-login handler), the
  auth middleware
  (`httpmw/middleware.go`), and the plugin SSO bridge (`backendapp/auth.go`)
  resolve the cookie name from the request via a new request-aware service
  method `CookieNameForRequest(r)`. The config default for `auth.cookieName`
  becomes **empty** (the service keeps the `kandev_session` base fallback, so
  the effective name is unchanged for default deployments); a non-empty
  configured value is returned verbatim, an empty one is port-suffixed. The
  gateway production code does **not** read the session cookie (it only
  handles the path-scoped `kandev_port_proxy` capability); its
  `svc.CookieName()` uses are test request headers in
  `gateway/websocket/port_proxy_auth_test.go` and must be updated to the
  scoped name for their request Host.
- **Workspace cookie scoping** (`backendapp/boot_state_routes.go`):
  `readActiveWorkspaceCookie` reads the scoped names first, then the legacy
  unprefixed names. All downstream `firstValidID` validation is unchanged.
  The suffix comes from the request Host (XFH precedence) — the same port
  the SPA derives from the API base URL.

### Frontend

- `apps/web/lib/routing/route-bootstrap.ts`: add `scopedCookieName(name,
  port?)` / `readScopedCookie(name, port?)` — the port defaults to the API
  origin port from `getBackendConfig().apiBaseUrl` (not
  `window.location.port`, which mismatches in split-origin dev), and the
  explicit parameter makes the helpers pure and unit-testable without
  stubbing; legacy fallback. `readActiveWorkspaceCookie` and the office read
  path use it.
- `apps/web/components/app-sidebar/app-sidebar-workspace-navigation.ts`:
  `writeWorkspaceCookie` writes the scoped name (same API-origin port).
- Server-component reads that go through `readCookies()`/`cookieStore`:
  `apps/web/app/page.tsx`, `apps/web/app/office/layout.tsx`,
  `apps/web/app/settings/layout.tsx`,
  `apps/web/app/office/lib/get-active-workspace.ts` (not
  `lib/get-active-workspace.ts`), and the separate legacy read in
  `apps/web/src/office-routes.tsx` read the scoped name first, legacy
  fallback.

### Docs

- **Deployment note** (`docs/public/authentication.md`): multiple
  auth-enabled instances on one host are isolated per port; the proxy MUST
  preserve the browser **hostname** — either preserve the full `Host`
  (host:port) or rewrite only the port and forward a correct
  `X-Forwarded-Host` (the CORS/WS origin gate compares `Origin` to `Host`
  and ignores XFH, so rewriting a non-loopback hostname 403s before auth). A custom
  `auth.cookieName` does not solve or alter the origin gate (it compares
  hostnames, independent of cookie names) — such deployments must still use
  a compliant Host/Origin setup, and the custom name disables automatic port
  isolation (must be unique per cookie host). **Session-cookie
  migration**: old auth-enabled builds conflict with other old builds; an
  upgraded instance ignores the legacy session token and requires one
  re-login; workspace selections keep their validated legacy read fallback.
- **Stale references** (ADR 0050, ADR
  2026-07-24-opt-in-authentication, `apps/backend/AGENTS.md`,
  `apps/backend/internal/auth/service_sso.go`,
  `apps/backend/internal/plugins/auth_login.go`,
  `apps/backend/internal/auth/store/models.go`): wording that pins the
  literal `kandev_session` cookie name becomes "the session cookie (name
  derived from the request host)".
- Already amended with this package: `docs/specs/auth/requirements/auth.md`,
  `docs/decisions/0023-active-workspace-cookie.md`,
  `docs/public/plugins-authoring.md`.

## Tests

Regression tests that must fail before the change:

- **Backend, session cookie** (`auth/httpapi/handlers_test.go` +
  `httpmw/middleware_test.go`): login via a request with Host
  `127.0.0.1:8443` sets `kandev_session_8443`; a request carrying only the
  unprefixed `kandev_session` cookie is unauthenticated; a request carrying
  the scoped cookie authenticates. Pre-fix: only `kandev_session` exists, so
  the scoped assertions fail.
- **Backend, session set/clear coverage**: setup, login, logout, invite
  accept (there is no separate admin-login handler — the first admin's
  setup/login flows through `handlers.go`), and plugin SSO all set/clear the
  scoped name; a
  configured custom `auth.cookieName` stays verbatim (no suffix) on the same
  ported host. Gateway `port_proxy_auth_test.go` request headers use the
  scoped name for their Host.
- **Backend, config-default regression**: with the **loaded** default config
  (empty `auth.cookieName`), login on a ported Host still sets
  `kandev_session_<port>`; a seeded non-empty default must not suppress
  suffixing. Custom-name tests must exercise both an empty config and an
  explicitly configured name.
- **Backend, env contract**: `auth.cookieName` gets an explicit
  `BindEnv("auth.cookieName", "KANDEV_AUTH_COOKIE_NAME")` (the automatic
  camelCase mapping would otherwise expose the undocumented
  `KANDEV_AUTH_COOKIENAME`). Precedence tests: loaded default (empty),
  config.yaml custom value, env custom value, env-over-file.
- **Backend, conflicting Host/XFH**: resolver regression — Host
  `public.example:8443` + `X-Forwarded-Host: public.example:9443` yields
  `_9443` (XFH wins; pinned as the proxy-misconfiguration contract).
- **Backend, custom-name negative**: two routers on one cookie host with the
  **same** configured custom name still conflict (documented exception —
  custom names disable automatic port isolation); the test pins the
  documented behavior.
- **Backend, mixed-version coverage**: tier 1 failing-before — ported Host
  with a valid token under the unprefixed name 401s (old→new rejection);
  re-upgrade — login writes the scoped name and a scoped request
  authenticates. Tier 1b invariant back-compat guards — rollback (no scoped
  cookie, scoped request 401s, 401 both sides) and no-port unprefixed
  authentication (identical both sides) protect against post-change
  regressions. Tier 2 — old-old conflict and the old side's unprefixed
  success are old-build wire behaviors, documented in the spec and
  explicitly NOT faked by a synthetic legacy adapter.
- **Backend, legacy-absence**: every session set/clear path (setup, login,
  logout, invite accept, plugin SSO) asserts the exact
  Set-Cookie name set contains only the scoped name — no unprefixed
  `kandev_session` emitted; positive-only assertions would miss an
  implementation emitting both names.
- **Frontend, boot-reader coverage**: the four CookieStore readers
  (`app/page.tsx`, `app/office/layout.tsx`, `app/settings/layout.tsx`,
  `app/office/lib/get-active-workspace.ts`) must use
  `readScopedCookieStoreValue`; the sync office effect (`office-routes.tsx`)
  uses the document-cookie `readScopedCookie` (both share `scopedCookieName`).
  A structural guard test asserts both the absence of direct lookups —
  literal names **or** the exported constants — (negative) and the presence
  of each file's mandated helper reference (positive), so a boot path cannot
  silently stay unscoped (task 04).
- **Backend, plugin SSO bridge**: the real bridge (`backendapp/auth.go`)
  sets the scoped name for a ported Host; the existing fake-bridge test
  (`plugins/auth_login_test.go`, which hardcodes `kandev_session`) is not
  the only coverage.
- **Backend, two-instance isolation** (integration, `auth` package or
  `backendapp`): two routers with different request Hosts and independent
  stores; logging into A authenticates only `kandev_session_<portA>`; B
  rejects A's cookie and authenticates only its own; workspace cookie reads
  in `boot_state_routes.go` resolve per-Host with both scoped cookies in the
  jar (non-first fixtures, failing-before).
- **Backend, proxy contract**: full-router tests — the CORS/WS origin gate
  (`httpmw.AllowedOrigin`) compares `Origin` to `r.Host` hostnames and
  ignores ports and `X-Forwarded-Host`. Three cases: preserved `Host`
  (`public.example:8443`) reaches login — **reachability is invariant**
  (current code authenticates under a preserved Host), so the failing-before
  assertion is the paired `Set-Cookie: kandev_session_8443`; **same-hostname
  port rewrite** (`Host: public.example:38429` + `XFH: public.example:8443`)
  passes the gate — the failing-before assertion is the cookie resolver
  deriving `_8443` from XFH; **non-loopback hostname rewrite**
  (`Host: internal:38429`, a different non-loopback hostname) is rejected
  403 before auth — invariant branch. The 403 fixture uses a different
  non-loopback hostname, never a different port; loopback-alias rewrites
  pass the existing gate and are unchanged.
- **Backend, workspace cookie** (`backendapp/boot_state_workspace_test.go`):
  `readActiveWorkspaceCookie` with Host `127.0.0.1:8443` prefers
  `kandev-active-workspace_8443` over the unprefixed name, and falls back to
  the legacy name when no scoped one exists. Pre-fix: the scoped name is
  never read. **Family separation**: the general reader ignores the
  office-family cookies (pre-change the backend generic reader consults the
  legacy office name — fails pre-change); `readOfficeWorkspaceCookie` reads
  the office family only; backend/frontend parity tests.
  **Downstream regression** (scenario 3): through
  `bootPayload`/resolver — invariant/back-compat (valid unprefixed legacy id
  selected when no scoped cookie exists; foreign/invalid legacy id falls
  through to the instance-owned fallback) and failing-before
  (scoped-over-legacy precedence when both exist).
  **Office precedence**: canonical frontend resolver (route → general-if-office-valid → office → settings → first, with `getActiveWorkspaceId` passing its URL override; wrappers pass null only when none exists; convergence is client-eventual via the OfficeRoutes bootstrap effect; route is a client/page-only candidate — backend boot payload and server layout are cookie-first by design). The backend Go resolver maps the general/office/settings vectors (settings wired from `b.userSettings`), regression-tested through `officeWorkspaces`/`addOfficeRouteState` with distinct valid O1/O2, office-vs-settings, and office-absent cases; a downstream generic-route regression (every generic `RouteName` that resolves an active workspace id — Unknown, Home, Tasks, Settings, integrations, Stats — plus the quick-chat fallback, with only office cookies → settings/first; TaskDetail asserted separately via routeData/task-workspace, not the settings/first matrix) pins the family separation (tasks 03/04).
- **Frontend, cookie helpers** (`apps/web/lib/routing/route-bootstrap.test.ts`,
  `apps/web/components/app-sidebar/app-sidebar-workspace-navigation.test.ts`):
  `scopedCookieName(name, port)` is pure — tests pass `"8443"` explicitly and
  assert the `kandev-active-workspace_8443` read/write plus legacy fallback
  (no jsdom Location stubbing required). A split-origin regression asserts
  the default port comes from the API base URL (`:38429`) and not from
  `window.location.port` (`:37429`). **Legacy-absence**: the sidebar write
  tests (`app-sidebar-workspace-navigation.test.ts` and
  `app-sidebar-workspace-picker.test.tsx` — the picker drives the same
  `rememberWorkspaceSelection` path and today asserts unprefixed writes)
  assert `rememberWorkspaceSelection` does NOT write the unprefixed names
  (legacy read fallback stays; no new legacy writes).
- **E2E** (`apps/web/e2e/tests/...`): auth project asserts the login
  response sets `kandev_session_<port>`; the workspace-switch spec asserts
  `kandev-active-workspace_<port>` is written on selection. The Playwright
  fixture runs one backend per worker (no second backend), so the
  two-instance behavioral regression is proven by the Go integration test
  above; a two-backend Playwright fixture is explicitly out of scope.

## Implementation Waves And Parallel Candidates

Execution stays sequential in the primary conversation. Waves are a human
decision aid; only disjoint tasks are labelled parallel-safe.

Wave 1:

- [x] [task-01-shared-port-cookie-helper](task-01-shared-port-cookie-helper.md)
- [x] [task-04-workspace-cookie-port-scope-frontend](task-04-workspace-cookie-port-scope-frontend.md)

`task-01` (Go helper) and `task-04` (frontend) touch disjoint trees and
contracts; parallel-safe.

Wave 2:

- [x] [task-02-session-cookie-port-scope-backend](task-02-session-cookie-port-scope-backend.md)
- [x] [task-03-workspace-cookie-port-scope-backend](task-03-workspace-cookie-port-scope-backend.md)

Both consume `httpcookie` from `task-01`; disjoint files (auth vs
boot-state), parallel-safe after Wave 1.

Wave 3:

- [x] [task-05-e2e-cookie-scope-assertions](task-05-e2e-cookie-scope-assertions.md)
- [x] [task-06-docs-and-stale-references](task-06-docs-and-stale-references.md)

Task 05 depends on the shipped behavior of tasks 02 and 04. Task 06 (docs
only) is parallel-safe with 05.

## Validation commands

```bash
make fmt
cd apps/backend && go test ./internal/common/config/... ./internal/common/httpmw/... ./internal/auth/... ./internal/backendapp/... && make lint
cd apps && pnpm --filter @kandev/web test -- lib/routing/route-bootstrap.test.ts components/app-sidebar/app-sidebar-workspace-navigation.test.ts
cd apps/web && pnpm run typecheck && pnpm run lint
cd apps/web && pnpm e2e:raw --project=auth  # or the auth project's exact command
```

## Invariant-negative pairing

The non-loopback-hostname-rewrite 403 and same-custom-name-conflict tests are invariant
branches that pass before the fix too; pair each with a change-specific
assertion (the XFH suffix derivation, the scoped Set-Cookie name) so the
suite still fails pre-change where a scenario demands it.

## Risks

- The proxy contract is the trap: the CORS/WS origin gate compares `Origin`
  to `r.Host` hostnames and ignores ports and XFH, so the deployment rule is
  "the proxy MUST preserve the browser Origin **hostname**". Compliant:
  preserved `Host` (full host:port), or a same-hostname port rewrite with a
  `X-Forwarded-Host` carrying the browser host:port (the resolver takes
  the public port from XFH). Only a **non-loopback hostname** rewrite 403s
  before auth (loopback-alias rewrites like `localhost` → `127.0.0.1` pass
  the gate by existing `AllowedOrigin` design and stay unchanged); neither
  XFH nor `auth.cookieName` can pass the gate, and
  `AllowedOrigin` must not be weakened to trust XFH (CSRF boundary). When
  Host and XFH conflict on port, XFH wins for cookie naming (pinned by a
  resolver regression test).
- One-time re-login after upgrade for every auth-enabled instance. Accepted
  tradeoff; the alternative (legacy fallback) resurrects the cross-instance
  bleed during mixed-version periods.
- The `kandev_session` cookie name is referenced by gateway **tests**
  (`port_proxy_auth_test.go`); every read/set site must use the scoped name
  or WS/port-proxy auth tests break. Production gateway code only handles
  the path-scoped `kandev_port_proxy` capability cookie and needs no change.
- Changing the `auth.cookieName` config default to empty must not change the
  effective name for default deployments (the service keeps the base
  fallback); any fixture or docs that assert the config value itself needs a
  review (task 06 covers docs).
- Split-origin dev is the trap: any frontend port derivation not based on
  the API origin silently degrades to the legacy fallback. The split-origin
  regression test is the guard.
- Custom `auth.cookieName` values disable automatic port isolation; the
  documented exception and the negative test keep this from being
  rediscovered as a "fix".
