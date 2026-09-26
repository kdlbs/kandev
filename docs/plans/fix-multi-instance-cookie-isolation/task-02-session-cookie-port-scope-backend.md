---
id: "02-session-cookie-port-scope-backend"
title: "Port-scope the session cookie (backend)"
status: done
wave: 2
depends_on: ["01-shared-port-cookie-helper"]
plan: "plan.md"
spec: "../../specs/auth/requirements/fix-multi-instance-cookie-isolation.md"
---

# Task 02: Port-scope the session cookie (backend)

## Acceptance

- The auth service gains a request-aware name resolver,
  `CookieNameForRequest(r *http.Request) string`. The config default for
  `auth.cookieName` becomes **empty** (currently seeded to
  `kandev_session` in `common/config/config.go`); the service keeps the
  `kandev_session` base fallback internally so the effective name for
  default deployments is unchanged. A non-empty configured value is returned
  verbatim (never suffixed); an empty one is returned via
  `httpcookie.ScopedName(r, baseName)`. The request-less `CookieName()`
  stays for contexts without a request (tests).
- Every production set/clear/read site resolves the name from the request:
  setup, login, logout, **invite accept**, and the plugin SSO bridge
  (`auth/httpapi/handlers.go`, `handlers_admin.go` — invite accept; there is
  no separate admin-login handler — the first admin's setup/login flows
  through `handlers.go`), the auth middleware read
  (`auth/httpmw/middleware.go`), and the plugin SSO bridge
  (`backendapp/auth.go`). Gateway production code does **not** read
  the session cookie (only the path-scoped `kandev_port_proxy` capability);
  the `svc.CookieName()` uses in `gateway/websocket/port_proxy_auth_test.go`
  are test request headers and must use the scoped name for their Host.
- A request carrying only the legacy unprefixed `kandev_session` cookie is
  **not** authenticated (no fallback).
- Regression tests, failing pre-change:
  - `auth/httpapi/handlers_test.go`: login with Host `127.0.0.1:8443` sets
    `kandev_session_8443`; logout clears the scoped name; invite accept and
    setup (the first admin's login) set it; a subsequent authenticated call
    must carry the scoped cookie; the unprefixed cookie alone yields 401.
  - **Config-default regression**: with the **loaded** default config (empty
    `auth.cookieName`), login on the same ported Host still sets
    `kandev_session_8443`; a seeded non-empty default must not suppress
    suffixing. Custom-name tests cover both an empty config and an
    explicitly configured `auth.cookieName = "my_session"` (verbatim, no
    suffix).
  - Custom-name tests: parameterize the `apiClient` fixture's session-cookie
    capture (currently filters `strings.Contains(name, "kandev_session")`,
    which discards `my_session`); give the fixture a configurable expected
    prefix (default `kandev_session`) or capture the exact `Set-Cookie` name
    from the response, so a custom-name login authenticates its subsequent
    request and asserts verbatim behavior through the same middleware path.
  - **Env contract**: add `_ = v.BindEnv("auth.cookieName",
    "KANDEV_AUTH_COOKIE_NAME")` in `common/config/config.go` (AutomaticEnv's
    camelCase mapping would expose the undocumented `KANDEV_AUTH_COOKIENAME`);
    precedence tests: loaded default (empty), config.yaml custom, env custom,
    env-over-file.
  - Proxy contract: full-router tests, distinguishing hostname rewrite from
    port-only rewrite (the CORS gate compares hostnames and ignores ports):
    **(a) preserved Host** `public.example:8443` + `Origin:
    https://public.example:8443` reaches login and authenticates —
    **reachability is invariant** (current code authenticates under a
    preserved Host), so the failing-before assertion is the paired
    `Set-Cookie: kandev_session_8443` on the response; **(b)
    same-hostname port rewrite** — `Host: public.example:38429` +
    `X-Forwarded-Host: public.example:8443` + `Origin:
    https://public.example:8443` — passes the origin gate (hostname match),
    and the **failing-before assertion is the cookie resolver deriving
    `_8443` from XFH**; **(c) non-loopback
    hostname rewrite** — `Host: internal:38429` (a different non-loopback
    hostname) + `X-Forwarded-Host: public.example:8443` + `Origin:
    https://public.example:8443` — rejected 403 by the origin gate before
    auth (invariant branch). The 403 fixture must use a DIFFERENT
    non-loopback hostname, not a
    different port; loopback-alias rewrites (`localhost` → `127.0.0.1`) pass
    the gate by existing `AllowedOrigin` design and are kept unchanged (not
    regression-tested as a change).
  - Custom-name negative: two routers on one cookie host with the **same**
    configured custom name still conflict (documented exception: custom
    names disable automatic port isolation); the test pins the documented
    behavior.
  - Mixed-version coverage, split into a failing-before regression tier, an
    invariant back-compat tier, and a documented-limitation tier (a
    synthetic legacy adapter would test nothing real, so old-build wire
    behavior is not faked):
    - **Tier 1 — failing-before regressions** (must fail before / pass
      after): on a ported Host, a **valid** token carried under the
      unprefixed `kandev_session` name is rejected 401 (old→new rejection —
      pre-change the middleware reads the unprefixed name and authenticates);
      re-upgrade — login on a ported Host writes `kandev_session_<port>` and
      a scoped request authenticates (pre-change the scoped name is never
      read → 401). All exercised through the real handlers/middleware/stores.
    - **Tier 1b — invariant back-compat guards** (pass both sides; protect
      against post-change regressions, not proof of the change): rollback —
      no scoped cookie exists, so a scoped request 401s (401 both sides);
      no-port Host — the unprefixed token still authenticates (pre-change and
      post-change identical).
    - **Tier 2 — documented limitations (spec only, not regression tests)**:
      old-old conflict and the old side's unprefixed-session success are
      wire behaviors of old binaries that current code cannot exhibit; the
      spec documents them and this suite explicitly does not fake them.
  - **Legacy-absence assertions** (negative, fail-before): every session
    set/clear path — setup, login, logout, invite accept, plugin SSO —
    asserts the **exact Set-Cookie name set** contains only the
    scoped name (no unprefixed `kandev_session` emitted); a positive-only
    assertion would let an implementation emit both names and retain the
    cross-instance bleed.
  - Two-instance isolation integration test (two routers, independent
    stores, Hosts `127.0.0.1:8443` / `127.0.0.1:9443`): login on A
    authenticates only A; B rejects A's scoped cookie; login on B keeps A's
    session intact; each request authenticates only with its own port's
    cookie name.
  - `httpmw/middleware_test.go`: middleware resolves the scoped name; a
    foreign-port token (`kandev_session_9443`) is rejected.
  - **Plugin SSO bridge**: a **real bridge regression** — drive
    `LoginExternal` (or the plugin webhook relay path) through
    `backendapp/auth.go` with a Gin request Host `127.0.0.1:8443` and assert
    the response sets `kandev_session_8443`; plus a custom-name verbatim
    case. A resolver-output test alone is insufficient: a stale bridge call
    to `b.auth.CookieName()` would pass resolver tests while emitting the
    plain name for ported SSO. The existing fake-bridge test
    (`plugins/auth_login_test.go`, which hardcodes `kandev_session`) must
    not be the only coverage.
  - Default-port request path: Host `example.com` (no port) — login sets
    the plain `kandev_session`, the middleware authenticates with it, and
    logout clears it (the no-port branch exercised through real handlers,
    not just the helper).
  - `gateway/websocket/port_proxy_auth_test.go`: request headers carry
    `kandev_session_<port>` for the fixture Host.

## Verification

```bash
cd apps/backend && go test ./internal/common/config/... ./internal/common/httpmw/... ./internal/auth/... ./internal/backendapp/... ./internal/gateway/websocket/... && make lint
```

## Files likely touched

- `apps/backend/internal/common/config/config.go` (auth.cookieName default → empty; BindEnv `KANDEV_AUTH_COOKIE_NAME`)
- `apps/backend/internal/auth/service.go` (new `CookieNameForRequest`)
- `apps/backend/internal/auth/httpapi/cookie.go`
- `apps/backend/internal/auth/httpapi/handlers.go`
- `apps/backend/internal/auth/httpapi/handlers_admin.go`
- `apps/backend/internal/auth/httpapi/handlers_test.go`
- `apps/backend/internal/auth/httpmw/middleware.go`
- `apps/backend/internal/auth/httpmw/middleware_test.go`
- `apps/backend/internal/backendapp/auth.go` (plugin SSO bridge)
- `apps/backend/internal/gateway/websocket/port_proxy_auth_test.go`
- `apps/backend/internal/common/config/config_test.go` (default + env precedence)
- `apps/backend/internal/common/httpmw/origin_test.go` (proxy boundary, if it lives with the origin policy)

## Dependencies

`httpcookie` from task 01.

## Parallelism

Disjoint files from `task-03`; parallel-safe after Wave 1.

## Inputs

- Spec: `What` (session cookie scoping, `CookieNameForRequest`, no legacy
  fallback), `Scenarios` (first, fourth, fifth, proxy), `Failure modes`
  (proxy, mixed-version).
- Plan: `Backend > Session cookie scoping`, `Tests`.

## Risks

- Missing a set/clear site (invite accept and the plugin bridge were the
  gaps found in review) leaves that flow writing the unprefixed name; the
  acceptance list is exhaustive — grep `CookieName()`, `setSessionCookie(`,
  and `b.auth.CookieName` to prove no stragglers.
- The seeded non-empty config default is the trap: `CookieNameForRequest`
  must distinguish "empty config" (suffix the base name) from "explicitly
  configured" (verbatim). The loaded-config regression test guards this.
- The CORS/WS origin gate compares `Origin` hostnames to `r.Host` and
  ignores ports and XFH; do not weaken `AllowedOrigin` to trust XFH in this
  fix (CSRF boundary). The deployment contract is "proxy preserves the
  hostname": preserved Host passes, same-hostname port rewrite + correct
  XFH passes (resolver takes the public port from XFH), non-loopback
  hostname rewrite 403s (loopback aliases pass by existing design); the
  full-router test proves all three.
- Custom-name negative test pins the documented exception: same custom name
  on one cookie host = conflict by design.
- The custom-name rule must live in `CookieNameForRequest`, not the generic
  helper, or configured deployments silently get suffixed names.
- The logout/clear path must use the same scoped name as login.

## Output contract

Report the scoped-name coverage (every set/read site), the custom-name and
proxy tests, the two-instance integration test, changed files, exact
commands and results, blockers/risks, then mark this task `done` and update
`plan.md`.
