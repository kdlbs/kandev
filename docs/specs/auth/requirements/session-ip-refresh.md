---
status: active
system: auth
created: 2026-08-19
owners:
  - tbd
---
# Session IP Refresh Requirements

## Overview

A browser session's IP is written once at login and never updated. Sessions
can live for weeks (the sliding TTL is refreshed on activity), so the account
security page (`Settings > Account > Security`, backed by
`GET /api/v1/auth/sessions`) keeps showing the login-time address forever.
Roaming/mobile clients, DHCP re-leases, or a proxy that only started
forwarding `X-Forwarded-For` after login leave a stale IP on display, which
users read as a security problem (a proxy IP attributed to their client).

## Requirements

### REQ-AUTH-SESSION-IP-REFRESH-001: Session IP Refresh

**Intent:** A browser session's IP is written once at login and never updated. Sessions can live for
weeks (the sliding TTL is refreshed on activity), so the account security page (`Settings > Account
> Security`, backed by `GET /api/v1/auth/sessions`) keeps showing the login-time address forever.
Roaming/mobile clients, DHCP re-leases, or a proxy that only started forwarding `X-Forwarded-For`
after login leave a stale IP on display, which users read as a security problem (a proxy IP
attributed to their client).

#### Acceptance criteria

- **AC-AUTH-SESSION-IP-REFRESH-001.1:** The recorded session IP SHALL be refreshed, after creation, to the current request's resolved client IP when the two differ.
- **AC-AUTH-SESSION-IP-REFRESH-001.2:** The refresh SHALL ride the existing throttled session-touch path (`TouchSession`, interval 60 seconds — `sessionTouchInterval = time.Minute` in `internal/auth/service.go`; nominally at most once per minute per session; the throttle is best-effort — concurrent resolves that all read a pre-interval `LastSeenAt` each write, and the last write wins, exactly as the timestamp touch already behaves): no per-request writes are added.
- **AC-AUTH-SESSION-IP-REFRESH-001.3:** An empty or unresolvable client IP SHALL NEVER overwrite a recorded value.
- **AC-AUTH-SESSION-IP-REFRESH-001.4:** Login-time recording (and setup / invite-accept / plugin-SSO recording) SHALL remain unchanged: the IP captured when the session is minted is the starting value.
- **AC-AUTH-SESSION-IP-REFRESH-001.5:** The client IP used for the refresh is the same `ClientIP()` value that records the IP at login, including `KANDEV_TRUSTED_PROXIES` resolution (see [trusted-proxies.md](trusted-proxies.md)).
- **AC-AUTH-SESSION-IP-REFRESH-001.6:** Middleware-level conformance tests MUST clear gin's trusted proxies on their router (`SetTrustedProxies(nil)`, exactly as production's `configureTrustedProxies` does): a bare `gin.New()` trusts `X-Forwarded-For` from ANY peer, so an untrusted-peer test would resolve the forwarded header instead of the TCP peer and could be misread as evidence to wire the middleware to raw headers — the exact bug the untrusted-header scenario exists to catch.
- **AC-AUTH-SESSION-IP-REFRESH-001.7:** Conformance tests that READ BACK a session's IP over HTTP become time-sensitive under this refresh: a >60s gap between the login POST and the read-back lets the read-back request's own touch overwrite the asserted IP. Any such read-back MUST re-apply the login request's full transport closure (RemoteAddr + forwarded headers) so the touch is idempotent. This applies to the landed `TestLoginSessionIP*` suite in `httpapi/handlers_test.go` and to any new middleware-level read-back.
- **AC-AUTH-SESSION-IP-REFRESH-001.8:** The refresh applies to EVERY request the global middleware authenticates, including WebSocket upgrades and deferred-path routes (`/ws`, `/terminal/`, `/lsp/`, `/vscode/`, `/port-proxy/`, `/mcp`, SPA-shell; office-with-bearer — refresh applies only when the request ALSO carries a session cookie, since the global middleware never authenticates bearer-only office JWTs): `httpmw.Middleware` runs `ResolveRequest` before the `isDeferredPath` check, so cookie-authenticated requests to those paths touch too. A middleware-level conformance test MUST exercise a cookie-authenticated deferred-path request (e.g. `GET /ws`) and assert the stored IP refreshed — this pins the skip-resolve-for-deferred-paths regression.

## Migrated source detail

## Why

A browser session's IP is written once at login and never updated. Sessions
can live for weeks (the sliding TTL is refreshed on activity), so the account
security page (`Settings > Account > Security`, backed by
`GET /api/v1/auth/sessions`) keeps showing the login-time address forever.
Roaming/mobile clients, DHCP re-leases, or a proxy that only started
forwarding `X-Forwarded-For` after login leave a stale IP on display, which
users read as a security problem (a proxy IP attributed to their client).

## What

- The recorded session IP SHALL be refreshed, after creation, to the current
  request's resolved client IP when the two differ.
- The refresh SHALL ride the existing throttled session-touch path
  (`TouchSession`, interval 60 seconds — `sessionTouchInterval = time.Minute`
  in `internal/auth/service.go`; nominally at most once per minute per
  session; the throttle is best-effort — concurrent resolves that all read
  a pre-interval `LastSeenAt` each write, and the last write wins, exactly
  as the timestamp touch already behaves): no per-request writes are
  added.
- An empty or unresolvable client IP SHALL NEVER overwrite a recorded value.
- Login-time recording (and setup / invite-accept / plugin-SSO recording)
  SHALL remain unchanged: the IP captured when the session is minted is the
  starting value.
- The client IP used for the refresh is the same `ClientIP()` value that
  records the IP at login, including `KANDEV_TRUSTED_PROXIES` resolution
  (see [trusted-proxies.md](trusted-proxies.md)).
- Middleware-level conformance tests MUST clear gin's trusted proxies on
  their router (`SetTrustedProxies(nil)`, exactly as production's
  `configureTrustedProxies` does): a bare `gin.New()` trusts
  `X-Forwarded-For` from ANY peer, so an untrusted-peer test would resolve
  the forwarded header instead of the TCP peer and could be misread as
  evidence to wire the middleware to raw headers — the exact bug the
  untrusted-header scenario exists to catch.
- Conformance tests that READ BACK a session's IP over HTTP become
  time-sensitive under this refresh: a >60s gap between the login POST and
  the read-back lets the read-back request's own touch overwrite the
  asserted IP. Any such read-back MUST re-apply the login request's full
  transport closure (RemoteAddr + forwarded headers) so the touch is
  idempotent. This applies to the landed `TestLoginSessionIP*` suite in
  `httpapi/handlers_test.go` and to any new middleware-level read-back.
- The refresh applies to EVERY request the global middleware authenticates,
  including WebSocket upgrades and deferred-path routes (`/ws`,
  `/terminal/`, `/lsp/`, `/vscode/`, `/port-proxy/`, `/mcp`, SPA-shell;
  office-with-bearer — refresh applies only when the request ALSO carries
  a session cookie, since the global middleware never authenticates
  bearer-only office JWTs): `httpmw.Middleware` runs
  `ResolveRequest` before the `isDeferredPath` check, so cookie-authenticated
  requests to those paths touch too. A middleware-level conformance test
  MUST exercise a cookie-authenticated deferred-path request (e.g. `GET
  /ws`) and assert the stored IP refreshed — this pins the
  skip-resolve-for-deferred-paths regression.

## Data model

```text
auth_sessions
  ip  TEXT NOT NULL DEFAULT ''   existing column; updated in place on touch
```

No migration. The `ip` column already exists; only its write path changes.

## API surface

- HTTP surface is unchanged: `GET /api/v1/auth/sessions` keeps returning the
  session rows with their `ip` field; the value now tracks the client's
  current address instead of the login-time one.
- Internal Go contracts change (both `internal/auth` only):
  - `Service.ResolveSessionToken(ctx, token)` becomes
    `ResolveSessionToken(ctx, token, ip string)` — the caller passes the
    request's resolved client IP. Callers without a request IP (unit tests,
    non-HTTP resolvers) MUST pass `""`; an empty `ip` means "no IP refresh"
    (timestamps still touch). The mechanical test call sites pass `""`
    (service_test.go and service_sso_test.go).
  - `Store.TouchSession(ctx, id, lastSeen, expires)` becomes
    `TouchSession(ctx, id, lastSeen, expires, ip string)` — timestamps always
    update; `ip` refreshes the stored value only when non-empty.

## Failure modes

- **Empty client IP** (no peer address resolvable): the recorded IP is
  preserved; only timestamps update.
- **Touch write fails** (DB error): the resolution still succeeds — the
  session authenticates as today; the IP simply stays stale until the next
  successful touch. This matches the existing best-effort touch behavior.
- **IP differs but the touch interval has not elapsed**: the refresh is
  deferred to the next touch, which happens on the next authenticated
  request after the interval (if the client goes idle, that can be
  arbitrarily later — the throttle bounds WRITE FREQUENCY, not refresh
  latency). Writes stay at most once per minute per session; this is the
  bounded-write tradeoff the issue asks for.
- **Textual IP comparison**: the refresh gate compares `ClientIP()` strings
  verbatim (no canonicalization). A trusted proxy forwarding
  `X-Forwarded-For: ::ffff:2.2.2.2` DOES reach the comparison in mapped
  form — gin v1.9.1 returns raw header strings verbatim on the
  trusted-header path (`validateHeader`/`ClientIP`); `net.IP.String()`
  normalization applies only to the RemoteAddr fallback. A client
  presented once as `2.2.2.2` (non-mapped header) and later as
  `::ffff:2.2.2.2` therefore triggers one spurious rewrite, bounded to the
  touch interval and self-healing (the stored value becomes the new form);
  a session alternating between a mapped-form forwarded header and a plain
  RemoteAddr transport flips textual forms on successive touches, again
  bounded by the 60-second throttle.
  Accepted: the written value always comes from gin's `ClientIP()`, so no
  wrong address is ever recorded.

## Scenarios

- **GIVEN** a session created from IP `203.0.113.7` whose
  `last_seen_at` is older than the touch interval, **WHEN** an authenticated
  request arrives from IP `198.51.100.4`, **THEN** the stored session IP
  becomes `198.51.100.4` and `GET /api/v1/auth/sessions` returns
  `198.51.100.4`.
- **GIVEN** a session created from IP `203.0.113.7` whose
  `last_seen_at` is older than the touch interval, **WHEN** an authenticated
  request arrives from the same IP `203.0.113.7`, **THEN** the stored IP is
  unchanged (only timestamps update).
- **GIVEN** a session whose stored IP is `203.0.113.7`, **WHEN** an
  authenticated request with an empty client IP triggers the touch, **THEN**
  the stored IP stays `203.0.113.7`.
- **GIVEN** a session whose stored IP is empty (e.g. minted by a caller
  without a request IP), **WHEN** an authenticated request with a non-empty
  client IP triggers the touch, **THEN** the stored IP is backfilled with
  that client IP.
- **GIVEN** a freshly created session, **WHEN** an authenticated request
  arrives from a different IP before the touch interval elapses, **THEN** the
  stored IP is unchanged; it updates on the next request after the interval.
- **GIVEN** a login from IP `203.0.113.7`, **WHEN** the session is minted,
  **THEN** the recorded IP is `203.0.113.7` (login-time recording unchanged).

## Out of scope

- No UI changes: the security page keeps rendering the stored `ip` value.
- Personal access tokens are untouched (they carry no IP).
- No schema migration, no new configuration, no new endpoint.
- No E2E test: a browser cannot change its own client IP mid-session, and
  the user-visible surface is a stored value rendered verbatim; the behavior
  is covered by backend integration tests through the real middleware chain.
