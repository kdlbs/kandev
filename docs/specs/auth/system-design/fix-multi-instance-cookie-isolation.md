---
status: draft
system: auth
requirements:
  - REQ-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001
created: 2026-08-17
owners:
  - Kandev
---
# Isolate Kandev cookies between instances on one host System Design

## Purpose and boundaries

This design preserves the technical source detail for `REQ-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

Browsers match cookies by host, domain, and path; **port is not part of
cookie scope** (the `Secure` attribute controls transmission over TLS, not
scheme partitioning). Every kandev instance bound to the same host (same IP,
different ports) therefore shares one cookie jar: a cookie written by the
instance the user touched last is sent to every other instance on the next
request. `localStorage` and `sessionStorage` are origin-scoped (scheme +
host + port) and therefore port-segregated; cookies are the only browser
storage that leaks between instances (empirically verified: a cookie written
on port 18111 is readable on port 18222 of the same host, while
localStorage/sessionStorage are not). Known limit: instances served on the
same host at default ports over different schemes (`http://host` on :80 and
`https://host` on :443) both carry no port in their Host, so they keep the
plain cookie names and are not isolated by this fix; the reported scenario
(same scheme, different ports) is fully covered.

Three kandev cookies carry per-instance identity and are all written with
`Path=/` and no Domain/Port scoping:

- `kandev_session` — the auth session token (HttpOnly). Two auth-enabled
  instances on one host overwrite each other's token. The losing instance
  rejects the foreign token (per-instance session store), 401s every API
  call, and the SPA's `onUnauthorized` handler clears auth state and
  redirects to `/login`. With two or more instances this ping-pongs: every
  instance keeps breaking (`"crashes / blanks"` in the report). Reproduced
  live with two auth-enabled instances (A on :48429, B on :48529): logging
  into B yanked A's open UI to the Sign in page; each instance rejects the
  other's token with `{"mode":"enabled","authenticated":false}`. The
  maintainers documented the same caveat in kdlbs/kandev#2689 ("Cookies do
  not include ports in their scope. Two authenticated Kandev instances on the
  same hostname can have a `kandev_session` cookie conflict").
- `kandev-active-workspace` and `office-active-workspace` — the remembered
  active workspace. A foreign workspace id is read by every other instance at
  boot. Today both backend (`firstValidID`) and frontend resolvers validate
  the value against the instance's own workspace list and fall back to the
  first workspace, so this path is graceful rather than fatal — but it
  constantly churns which workspace (and, since ADR
  2026-08-15-office-mode-follows-active-workspace, which mode) each instance
  boots into, and any unvalidated read of these values would hard-fail.

Private-browsing the demo servers "fixes" the report because the browser
gives private contexts an isolated cookie partition — consistent with cookies
being the shared state.

## What

Port-scope the instance-identity cookie **names** so each instance reads and
writes only its own cookie. The browser already keeps all names in one jar;
each instance simply ignores the foreign-named ones.

- `kandev_session` becomes `kandev_session_<port>` where `<port>` is derived
  from the request Host (backend-side set and read), honoring
  `X-Forwarded-Host` when present (same pattern as the existing
  `X-Forwarded-Proto` TLS detection). A request without a port in its host
  keeps the plain name (`kandev_session`), so default-port deployments are
  unchanged. The backend resolves the name through a request-aware service
  method (`CookieNameForRequest(r)`). The config default for `auth.cookieName`
  is **empty** (the service keeps the `kandev_session` base fallback
  internally, so the effective name for default deployments is unchanged):
  when the configured value is non-empty it is returned verbatim (never
  suffixed); when empty the default base name is port-suffixed. This lets the
  resolver distinguish explicit configuration from the default — a seeded
  non-empty default would defeat suffixing in production. The generic
  `httpcookie.ScopedName` helper only appends the suffix and is never fed a
  configured name.
- `kandev-active-workspace` / `office-active-workspace` become
  `kandev-active-workspace_<port>` / `office-active-workspace_<port>`.
  **Both sides derive the suffix from the API origin port** (one canonical
  port): the SPA takes it from `getBackendConfig().apiBaseUrl` — equal to
  `window.location.port` in same-origin deployments (Go serves SPA + API on
  one port) and equal to the backend port in split-origin dev (Vite SPA on
  the web port, API on the backend port, no Vite proxy — the browser calls
  the backend directly), which is the same port the backend sees on its
  request Host; the backend derives it from the request Host
  (`X-Forwarded-Host` precedence). Deriving from `window.location.port`
  alone would mismatch the backend in split-origin dev. The unprefixed
  legacy names remain readable as a fallback (their values are still
  validated against the workspace list), so a pre-upgrade selection
  survives; new writes only use the scoped names.
- The session cookie has **no** legacy fallback: after upgrade each instance
  requires one re-login. A fallback would re-open the cross-instance bleed
  for any mixed-version period.
- **Workspace isolation scope: all-new builds.** The validated legacy
  fallback means an upgraded instance still reads the unprefixed workspace
  cookie while **old builds keep writing it**, so during a mixed-version
  deployment an old instance can overwrite the legacy value a new instance
  reads (when the id validates against the new instance's workspace list).
  Workspace-selection isolation therefore applies once **all** instances on
  the host are upgraded; the legacy fallback exists so pre-upgrade
  selections survive, and it is safe because resolution validates against
  the instance's own workspace set (a foreign id falls back gracefully).
  Sessions are isolated even during mixed versions (no legacy session
  fallback).

Observable behavior: with two or more **upgraded** kandev instances on one
host (same IP, different ports), logging into one no longer logs the others
out, and selecting a workspace in one no longer changes what the others boot
into. Single-instance and default-port behavior is unchanged. No HTTP
endpoint, WebSocket protocol, or data contract changes.

## API surface

Cookie names are browser-observable (DevTools, response `Set-Cookie`
headers): `kandev_session_<port>`, `kandev-active-workspace_<port>`,
`office-active-workspace_<port>`. No API, route, or wire change.

## Failure modes

- **Proxy drops `X-Forwarded-Host` or rewrites a non-loopback hostname.**
  Two distinct failures: (1) when the effective host the backend derives the
  suffix from differs from the browser-visible origin (missing or incorrect
  `X-Forwarded-Host` under a port rewrite), the cookie name mismatches and
  every request 401s (login loop); (2) when the proxy rewrites a
  **non-loopback** hostname, the CORS/WS origin gate rejects the request
  with 403 before auth (loopback-alias rewrites such as `localhost` →
  `127.0.0.1` pass the existing gate and are unchanged). **Primary
  deployment contract: the proxy MUST preserve the browser Origin
  hostname.** The CORS/WS origin gate (`httpmw.AllowedOrigin`) compares the
  browser `Origin` hostname with `r.Host` and does not consult
  `X-Forwarded-Host`, so a proxy that rewrites the **non-loopback hostname**
  is rejected with 403 before auth regardless of cookie naming;
  `X-Forwarded-Host` must
  carry the browser host:port for cookie naming. Two compliant setups:
  (a) preserve `Host` fully (XFH absent or identical); (b) rewrite only the
  port (same hostname) and forward a correct `X-Forwarded-Host` — CORS
  hostname (ports are not compared) while the cookie resolver takes
  the public port from XFH. **When both Host and X-Forwarded-Host carry
  conflicting ports, `X-Forwarded-Host` wins for cookie naming** (it
  represents the original browser Host across the chain); that conflict is a
  proxy misconfiguration and is pinned by a resolver regression test.
  Rewritten-**hostname** deployments require an independently configured
  CORS trust extension, which is out of scope here — **qualified to
  non-loopback hostnames**: `AllowedOrigin` already permits any origin/request
  pair whose normalized hosts are both loopback (e.g. `localhost` →
  `127.0.0.1`), an existing exception for dev reverse proxies that this fix
  keeps unchanged and does not regression-test as a change. A full-router
  test drives the boundary: preserved `Host` + public `Origin` reaches
  login; same-hostname port-rewrite + correct XFH passes and scopes to the
  public port; rewritten **non-loopback hostname** + public
  `X-Forwarded-Host` + public `Origin` is rejected 403.
- **Port in URL vs port in Host disagree** (rare; browsers normalize
  default ports out of Host). Suffix only when the Host actually carries a
  port, so `http://host` and `http://host:80` agree in practice.
- **IPv6 hosts.** `net.SplitHostPort` handles bracketed forms
  (`[::1]:8080`); unbracketed `::1` has no port and gets no suffix.
- **Custom cookie names disable automatic port isolation.** A non-empty
  `auth.cookieName` is used verbatim (never suffixed), so two auth-enabled
  instances on one cookie host configured with the **same** custom name
  overwrite each other exactly like the original bug — the isolation promise
  applies to default-named instances. Deployments that set a custom name
  must make it unique per cookie host (per instance), and should treat it as
  single-instance-oriented. The canonical environment override is
  `KANDEV_AUTH_COOKIE_NAME` (explicit `BindEnv`; the automatic camelCase
  mapping would otherwise yield the undocumented `KANDEV_AUTH_COOKIENAME`).
- **Mixed-version migration.** Old builds keep writing the unprefixed names.
  The upgraded instance ignores legacy session cookies (clean one-time
  re-login) and reads legacy workspace cookies only as a validated fallback,
  but an **old build still conflicts with other old builds, and a rollback
  resurrects the original bleed** (old binaries read/write the shared
  unprefixed `kandev_session`). Operational contract: all auth-enabled
  instances on one host must upgrade together; after a rollback every
  instance needs a fresh login. On re-upgrade the guarantee is mechanical:
  the new build reads only scoped cookie names, which the old build never
  wrote, so no valid scoped session exists until the user logs in again (old
  session rows remain in the DB but are unreachable via the scoped name). The
  upgraded instance does not proactively
  delete the legacy cookie — scrubbing the shared jar from one instance would
  just log other (old) instances out and they would re-write it.

## Scenarios

- **GIVEN** two auth-enabled kandev instances A (port 8080) and B (port
  8081) on the same host, **WHEN** the user logs into B, **THEN** A's open
  session keeps working, and A's next page load shows A's authenticated UI
  rather than the Sign in page.
- **GIVEN** the same pair, **WHEN** the user selects a workspace in B,
  **THEN** A's next boot resolves A's own workspace selection, not B's
  workspace id.
- **GIVEN** an instance upgraded from a version that wrote the unprefixed
  workspace cookie, **WHEN** the instance boots, **THEN** the legacy value is
  honored when it names one of the instance's own workspaces.
- **GIVEN** an instance upgraded from a version that wrote the unprefixed
  session cookie, **WHEN** the user loads the instance, **THEN** the user is
  asked to sign in once (the legacy token is not accepted), and the new
  scoped cookie is used afterwards.
- **GIVEN** a single instance on the default port (no port in the URL),
  **WHEN** the user logs in, **THEN** the cookie keeps the plain
  `kandev_session` name and the flow is unchanged.
- **GIVEN** two auth-enabled instances A and B on one host where B is still
  an old build, **WHEN** both users are logged in, **THEN** the old build
  still conflicts with other old builds (documented limitation — old-build
  wire behavior, not regression-tested).
- **GIVEN** the same pair after B upgrades and all instances re-login,
  **THEN** the conflict is gone — recovery is inferred from independently
  tested invariants (the new build rejects the legacy session token; two new
  builds keep isolated sessions per port).
- **GIVEN** an old instance A and an upgraded instance B on one host,
  **WHEN** the user selects a workspace in A, **THEN** B may read A's legacy
  unprefixed workspace value (validated fallback — resolved only when it
  names one of B's own workspaces, else B falls back); workspace isolation
  is a property of all-new-build deployments (documented limitation,
  invariant-tested).
- **GIVEN** an auth-enabled instance behind a reverse proxy that preserves
  the browser `Host` (`public.example:8443`), **WHEN** the user logs in,
  **THEN** the session cookie is set under the public-origin port suffix
  (`kandev_session_8443`) and subsequent requests authenticate.
- **GIVEN** an auth-enabled instance behind a reverse proxy that **rewrites
  a non-loopback hostname** (even with correct `X-Forwarded-Host`), **WHEN**
  the browser sends a request from the public origin, **THEN** the CORS/WS
  origin gate rejects it with 403 before auth (the gate compares `Origin` to
  `Host` and ignores `X-Forwarded-Host`); the cookie-name resolver would
  derive the `X-Forwarded-Host` suffix in isolation, but the request never
  reaches it. Rewritten-hostname deployments require an independently
  configured CORS trust extension, which is out of scope for this fix.
  Loopback-alias rewrites (e.g. `localhost` → `127.0.0.1`) pass the gate by
  existing `AllowedOrigin` design (dev reverse proxies) and are unchanged. A
  same-hostname port-rewrite with correct XFH remains compliant.
- **GIVEN** split-origin dev (SPA on the web port, API on the backend port,
  no Vite proxy), **WHEN** the user selects a workspace in the SPA, **THEN**
  the workspace cookie is written with the backend port suffix and the
  backend's boot reads the same scoped name (no legacy fallback needed).
- **GIVEN** an unconfigured (default) deployment on a ported host, **WHEN**
  the user logs in, **THEN** the session cookie is `kandev_session_<port>`
  — the empty config default must not suppress suffixing.
- **GIVEN** two auth-enabled instances on one cookie host both configured
  with the same custom `auth.cookieName`, **WHEN** both users are logged in,
  **THEN** the instances still conflict (documented exception: custom names
  disable automatic port isolation and must be unique per host).

## Out of scope

- `kandev_locale` (validated, benign) and the write-only `layout:*` cookies
  keep their names.
- The path-scoped port-proxy capability cookie (`kandev_port_proxy`) is
  unchanged.
- Personal access tokens and the WS `?token=` path are unaffected.
- No legacy fallback for the session cookie (deliberate; see What).
