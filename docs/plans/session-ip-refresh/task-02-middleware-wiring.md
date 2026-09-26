---
id: "02-middleware-wiring"
title: "Wire ClientIP through the auth middleware"
status: done
wave: 2
depends_on: ["01-store-service-refresh"]
plan: "plan.md"
spec: "../../specs/auth/requirements/session-ip-refresh.md"
---

# Task 02: Wire ClientIP through the auth middleware

## Acceptance

- `httpmw.ResolveRequest` passes `c.ClientIP()` to the new
  `svc.ResolveSessionToken(ctx, cookie, ip)` signature — the only production
  call site.
- The stale `ResolveRequest` doc comment (`httpmw/middleware.go:57-58`,
  "Shared with the WS gateway's upgrade-time check") is corrected — no WS
  caller of `ResolveRequest` exists; the gateway consumes the
  middleware-resolved identity and PATs via `AuthPolicy.ResolveToken`.
- A full-chain integration test proves the observable contract with four
  sub-cases: (a) a session aged past the touch interval with stored IP
  `1.1.1.1`, requested through the real gin middleware with the session
  cookie and `RemoteAddr` `2.2.2.2:1234` (no forwarded headers, trusted
  proxies cleared), ends with `svc.ListSessions` reporting `2.2.2.2` AND a
  real-route `GET /api/v1/auth/sessions` read-back (via
  `authhttpapi.RegisterRoutes`) asserting `rec.Code == http.StatusOK`
  FIRST, then that the JSON `ip` field is `2.2.2.2`; (b)
  with trusted proxies `10.0.0.0/8`, `RemoteAddr` `10.0.0.5:1234`,
  `X-Forwarded-For: 2.2.2.2`, AND `X-Real-IP: 9.9.9.9`, the stored IP is
  `2.2.2.2` — discriminating `c.ClientIP()` (with trusted-proxy
  resolution and XFF-over-X-Real-IP precedence) from a RemoteAddr read
  (stores `10.0.0.5`), an X-Real-IP read (stores `9.9.9.9`), and
  header-preference-order bugs; (c) on the DEFAULT router (proxies
  cleared), `RemoteAddr` `10.0.0.5:1234` with `X-Forwarded-For: 2.2.2.2`,
  the stored IP is `10.0.0.5` — discriminating `c.ClientIP()` (which
  ignores the untrusted header) from a raw `X-Forwarded-For` read, which
  would store `2.2.2.2`. Together (b)+(c) pin the exact trusted-proxy
  semantics of `ClientIP()`: trusted peer → header, untrusted peer →
  RemoteAddr. (d) a DEFERRED-path request (`GET /ws`; the unregistered
  route 404s — the middleware never 401s a deferred path and this router
  registers no gateway route, so the status is NOT a resolution proof; the
  `svc.ListSessions` IP assertion below is the real discriminator, since
  only a successful resolve touches the session row — note the PRODUCTION
  gateway's `requireConnectionAuth` does 401 unresolved deferred requests,
  so this scoped claim is test-router-only)
  with the cookie and `RemoteAddr` `2.2.2.2:1234` still refreshes —
  `ResolveRequest` runs before the `isDeferredPath` check, so
  cookie-authenticated deferred requests touch; this closes the
  skip-resolve-for-deferred-paths regression, the only gap in the planned
  discrimination matrix.
- The trusted-proxies login-time suite `TestLoginSessionIP*` in
  `httpapi/handlers_test.go` stays green deterministically: its
  `loginSessionIP` read-back GET re-applies the login request's FULL
  transport mutator closure (RemoteAddr + `X-Forwarded-For` + `X-Real-IP`),
  so the new touch-based refresh is idempotent on that read in all six
  cases and the suite gains no time-based assertion. Today's helper passes
  NO transport to the read-back GET (`handlers_test.go:135`), so the
  httptest default RemoteAddr `192.0.2.1:1234` would overwrite the
  asserted IP in ALL SIX cases on a >60s gap — the full closure covers
  every case. Reusing only the
  RemoteAddr would STILL leave the three forwarded-header cases
  (`UsesForwardedFor`, `UsesXRealIP`, `MalformedXFFFallsBackToXRealIP`)
  flaky: their stored IP is
  the header value `203.0.113.7`, and a GET carrying only the RemoteAddr
  against the trusted fixture resolves `10.0.0.5` — a >60s gap would
  overwrite the asserted IP.
- The full `internal/auth` test suite passes with the middleware wired.

## Files likely touched

- `apps/backend/internal/auth/httpmw/middleware.go` — pass `c.ClientIP()` in
  `ResolveRequest`.
- `apps/backend/internal/auth/httpmw/middleware_test.go` — `newTestService`
  also returns the auth store (mechanical update of existing callers);
  `newTestRouter` takes a variadic trusted-proxy list and clears them by
  default (`SetTrustedProxies(nil)`, mirroring production); new
  `TestSessionIPRefreshedThroughMiddleware` (sub-cases a-d above).
- `apps/backend/internal/auth/httpapi/handlers_test.go` — the
  `loginSessionIP` helper's `GET /api/v1/auth/sessions` re-applies the login
  request's FULL transport mutator closure (RemoteAddr + `X-Forwarded-For` +
  `X-Real-IP`) for an idempotent read-back touch. RemoteAddr-only would
  reintroduce the >60s flake in the three forwarded-header cases
  (`TestLoginSessionIPTrustedPeerUsesForwardedFor`,
  `TestLoginSessionIPUsesXRealIP`,
  `TestLoginSessionIPMalformedXFFFallsBackToXRealIP`); with NO transport
  at all today (default `192.0.2.1:1234`) all six cases flake — the full
  closure fixes every case.

## TDD sequence

1. RED: extend the httpmw harness — `newTestService` returns the auth store
   too; `newTestRouter` accepts a variadic trusted-proxy list (no args →
   `nil`, mirroring production; this flip from gin's trust-all default is
   behavior-preserving for the existing callers because none of the current
   tests sends `X-Forwarded-For`/`X-Real-IP` — verify that stays true when
   adding headers to an existing test) — and add
   `TestSessionIPRefreshedThroughMiddleware` with four sub-cases. Build
   the service with `newTestService(t, true)` (auth ENABLED): `false` puts
   the middleware in `ModeDisabled`, which short-circuits to
   `SyntheticIdentity` BEFORE `ResolveRequest` runs (httpmw/middleware.go)
   — the aged session is never resolved, every sub-case fails RED and STAYS
   RED after the correct `c.ClientIP()` wiring, a permanently-red test
   masquerading as a wiring defect. (The "do NOT call setupAdmin" note
   below only works because auth is enabled and `ResolveRequest` actually
   runs.) Each sub-case uses
   FRESH aged session created via `auth.GenerateSessionToken()` +
   `authstore.CreateSession` (the import alias used by `middleware_test.go`
   — the package is `httpmw`, not `auth`): `CreatedAt: now`, `LastSeenAt` at
   least 2 minutes in the past (the 1-minute `sessionTouchInterval` is
   unexported in package `auth`, so the recipe cannot name it from this
   package), `ExpiresAt: now.Add(24*time.Hour)` — a narrow future expiry
   (e.g. 1m; a 1h expiry would only hazard a >1h stall) would let a >60s
   creation-to-probe gap delete the session
   in `ResolveSessionToken` (`service_credentials.go:85-88`, delete before
   touch) and fail RED for the wrong reason, the same failure class
   task-01's 24h futures pin (aged and within-interval cases); the
   fixture's `SessionTTLHours = 720` makes
   the value consistent — `UserID: userstore.DefaultUserID` (active row
   seeded by `userstore.Provide` — `internal/user/store/sqlite.go:99-120`,
   `ensureDefaultUser` func at `:99`, INSERT at `:108-110`),
   `IP: "1.1.1.1"` (set `CreatedAt: now` — see above — for well-formed
   rows). Do NOT call `setupAdmin` for this test: the seeded
   active `DefaultUserID` row is sufficient, and `setupAdmin` would mint a
   SECOND `DefaultUserID` session that any single-row assumption would trip
   on (the TokenHash selection below handles it). Each sub-case runs on a
   FRESH harness — own service, router, and aged session; do not share one
   fixture across sub-cases. With a fresh fixture `ListSessions` contains
   only the created session; still assert by `TokenHash` (see below), never
   by `len(...) == 1` or index.
   Every `svc.ListSessions` assertion MUST select the row by `TokenHash`
   (or the session ID captured at creation), never by list index — the
   rule holds even though each sub-case has a single fresh session, so a
   future fixture extension cannot silently break the assertions.
   - Sub-case a: default router (trusted proxies cleared), cookie +
     `req.RemoteAddr = "2.2.2.2:1234"`, no `X-Forwarded-For` → assert
     `svc.ListSessions` reports `2.2.2.2`. Additionally, register the REAL
     auth routes on the router (`authhttpapi.RegisterRoutes(router, svc,
     log)` where `log, _ := logger.NewFromZap(zap.NewNop())`) —
     importable from this internal `httpmw` test without an import cycle,
     because `httpapi`'s non-test files never import `httpmw`; pass a real
     Nop-backed logger rather than nil so a future logging handler cannot
     nil-panic this integration test — and
     `GET /api/v1/auth/sessions` with the same cookie and transport
     closure, asserting `rec.Code == http.StatusOK` FIRST (the
     `expires=now` bug surfaces as a 401 here — assert it loudly at the
     status line, not via a failed JSON decode), then that the JSON row
     with `current: true` carries `ip: 2.2.2.2` (production
     `{"sessions":[{...}]}` wire shape; select by
     `current` flag, matching `loginSessionIP`'s selection in
     `httpapi/handlers_test.go` — robust if the fixture ever gains rows).
     EXECUTION ORDER is part of the contract: probe request FIRST (assert
     its 200), THEN `svc.ListSessions`, THEN the read-back. The read-back
     must run AFTER the probe's touch so its resolve observes a buggy
     `expires=now` — the probe writes `expires=now1` and the read-back's
     expiry check 401s at its JSON assertion. In the reversed order the
     same bug fails the PROBE's 200 assert instead (the read-back's touch
     writes `expires=now1`, the probe's resolve sees it expired and 401s)
     — red either way, but the pin keeps the discrimination in the
     read-back where the JSON assertion lives.
     This real-route read-back is ALSO the sole service-level
     `expires_at`-arg discrimination: a bug writing
     `expires = now` instead of `now + SessionTTL()` surfaces here as
     delete-on-expiry (the read-back's resolve sees `ExpiresAt.Before(now)`
     and returns 401) — it must NOT be removed as redundant with the
     `ListSessions` assertion. Using the
     real handler instead of a hand-mirrored route makes scenario 1's HTTP
     half genuinely end-to-end and immune to mirror drift (a renamed JSON
     tag would slip a mirror but not the real handler).
   - Sub-case b (trusted-peer discrimination): a router with
     `SetTrustedProxies([]string{"10.0.0.0/8"})`, cookie + `RemoteAddr`
     `10.0.0.5:1234` + `X-Forwarded-For: 2.2.2.2` + `X-Real-IP: 9.9.9.9` →
     assert `svc.ListSessions` reports `2.2.2.2`. The third header pins
     gin's header-PREFERENCE order (XFF wins over X-Real-IP), so a
     RemoteAddr-wired middleware (stores `10.0.0.5`) and an
     X-Real-IP-wired middleware (stores `9.9.9.9`) fail deterministically
     instead of by accident. Within-XFF element order is NOT pinned (a
     single XFF value is used, so a right-to-left parser returns the same
     value) — out of scope for this test.
   - Sub-case c (untrusted-header discrimination): the DEFAULT router —
     the `SetTrustedProxies(nil)` default from `newTestRouter`, NOT
     gin.New()'s trust-all default, which would honor the header, resolve
     `2.2.2.2`, and fail the case against correct wiring — cookie +
     `RemoteAddr` `10.0.0.5:1234` + `X-Forwarded-For: 2.2.2.2` → assert
     `svc.ListSessions` reports `10.0.0.5`. A middleware that reads the raw `X-Forwarded-For` header
     without the trusted-proxy check (would store `2.2.2.2`) fails this
     case. (With a RemoteAddr fallback such a middleware would sail through
     both (a) and (b), so (c) is required; a pure header reader with no
     fallback would already fail (a), where no header is set — either
     variant is caught.) Mirrors
     `TestLoginSessionIPUntrustedPeerIgnoresForwardedFor`
     (`httpapi/handlers_test.go`). Only `c.ClientIP()` passes (b)+(c),
     pinning the spec's "same `ClientIP()` value that records the IP at
     login, including `KANDEV_TRUSTED_PROXIES` resolution".
   - Sub-case d (deferred-path refresh): the DEFAULT router, a fresh aged
     session, cookie + `req.RemoteAddr = "2.2.2.2:1234"`, request
     `GET /ws` (or any `isDeferredPath` route) → assert
     `rec.Code == http.StatusNotFound` (the middleware never 401s a
     deferred path and this router registers no gateway route, so an
     unresolved request also 404s through the deferral — the status is
     NOT a resolution proof; production's gateway
     `requireConnectionAuth` would 401 here, so this scoped claim is
     test-router-only), then assert `svc.ListSessions` reports
     `2.2.2.2` — the IP assertion is the REAL discriminator, since only a
     successful resolve touches the session row.
     `ResolveRequest` runs before the deferred-path check, so a
     cookie-authenticated deferred request must touch; a buggy
     skip-resolve-for-deferred-paths optimization fails this case (the
     only planned-test gap it would otherwise slip through).
   RED: the middleware passes `""` (wired mechanically in Task 01, which
   updated the call to the three-arg signature), so the empty IP never
   refreshes and the stored IP stays `1.1.1.1` in all four sub-cases —
   assertion-level failures (`got 1.1.1.1; want 2.2.2.2 / 2.2.2.2 /
   10.0.0.5 / 2.2.2.2` for (a)/(b)/(c)/(d)).
2. Implement: `svc.ResolveSessionToken(ctx, cookie, c.ClientIP())` and
   correct the stale `ResolveRequest` doc comment (`middleware.go:57-58`).
3. Isolation hardening (keeps the landed trusted-proxies suite
   deterministic): make the `loginSessionIP` helper in
   `httpapi/handlers_test.go` capture the login request's FULL transport
   mutator closure (RemoteAddr + `X-Forwarded-For` + `X-Real-IP`) and
   re-apply it to the `GET /api/v1/auth/sessions` read-back. Reusing only
   the RemoteAddr is insufficient: in `TestLoginSessionIPTrustedPeerUsesForwardedFor`,
   `TestLoginSessionIPUsesXRealIP`, and
   `TestLoginSessionIPMalformedXFFFallsBackToXRealIP` the stored IP comes
   from a forwarded header (`203.0.113.7`), and a GET carrying only the
   RemoteAddr against the trusted fixture resolves `10.0.0.5` — a >60s gap
   would overwrite the asserted IP, the exact flake this hardening exists to
   remove. Re-applying the whole closure makes the touch idempotent in all
   six cases.
4. GREEN: the integration test passes; run the trusted-proxies suite and the
   full auth suite (verification block below).

## Verification

```bash
cd apps/backend && go test ./internal/auth/httpmw -run '^TestSessionIPRefreshedThroughMiddleware$' -count=1 && go test ./internal/auth/httpapi -run '^TestLoginSessionIP' -count=1 && go test ./internal/auth/... -count=1
```

## Dependencies

Task 01 (`01-store-service-refresh`) — this task uses the new
`ResolveSessionToken(ctx, token, ip)` signature and the middleware already
compiles against it with a `""` argument.

## Parallelism

`sequential`.

## Inputs

- Spec **Scenarios** 1 and **What** (refresh on the touch path, keyed on the
  request's `ClientIP()`, including `KANDEV_TRUSTED_PROXIES` resolution).
- Plan "Backend → Middleware" section.
- `apps/backend/internal/auth/httpmw/middleware_test.go:21-81`
  (`newTestService`, `newTestRouter`, `doRequest` — the latter already
  variadic over request mutators: set `RemoteAddr` and headers there); the
  probe route at
  `/api/v1/probe` echoes the resolved identity and returns 200 for a valid
  session. The auth store import is aliased `authstore`; the
  `sessionTouchInterval` constant is NOT accessible from this package.
- `auth.GenerateSessionToken()` returns `(token, hash, error)` — use `hash`
  for `authstore.CreateSession` and `token` as the cookie value.
- For the real-route read-back, import
  `github.com/kandev/kandev/internal/common/logger` (aliased `logger`,
  collision-free in `middleware_test.go`) and `go.uber.org/zap`
  (`logger.NewFromZap(zap.NewNop())` in `internal/common/logger`).
- `apps/backend/internal/auth/httpapi/handlers_test.go` — `loginSessionIP`
  (helper at `:112`; the read-back GET to harden is `:135`, `current`-flag
  selection ~`:150`) and `apiClient.do`'s variadic request mutator (added
  by the trusted-proxies work) for the idempotent read-back change.

## Risks

- `httptest.NewRequest` defaults `RemoteAddr` to `192.0.2.1:1234`; sub-case
  a must override it explicitly and must NOT set forwarded headers; sub-case
  b must set both the trusted list and the forwarded header, or the
  discrimination assertion is meaningless.
- `newTestService` currently returns `*auth.Service`; changing it to also
  return the store touches every existing test in the file — mechanical,
  keep assertions unchanged.
- Aged test sessions need a future `ExpiresAt` and the seeded active
  default-user row, or resolution fails before the touch path runs.
- Without the step-3 `loginSessionIP` hardening (full transport mutator,
  not just RemoteAddr), the landed `TestLoginSessionIP*` suite becomes
  time-sensitive under the new refresh behavior; the hardening is part of
  this task, not optional.

## Output contract

Report the RED failures (all four sub-cases), GREEN result, exact files changed,
remaining risks, and task/plan status sync. Record every exact command and
outcome in `## Results`.

## Results

RED (TDD step 1): `go test ./internal/auth/httpmw -run '^TestSessionIPRefreshedThroughMiddleware$' -count=1` → all 4 sub-cases fail at the assertion level with the mechanical `""` from Task 01:
- `cleared_proxies_uses_RemoteAddr`: `stored IP = "1.1.1.1", want 2.2.2.2`
- `trusted_peer_honors_X-Forwarded-For_over_X-Real-IP`: `stored IP = "1.1.1.1", want 2.2.2.2`
- `untrusted_peer_ignores_X-Forwarded-For`: `stored IP = "1.1.1.1", want 10.0.0.5`
- `deferred_path_request_still_refreshes`: `stored IP = "1.1.1.1", want 2.2.2.2`

GREEN (TDD step 2): `svc.ResolveSessionToken(ctx, cookie, c.ClientIP())` wired + stale doc comment corrected → the same command passes all 4 sub-cases.

Isolation hardening (TDD step 3): `loginSessionIP` in `httpapi/handlers_test.go`
re-applies the login request's full transport closure (RemoteAddr +
X-Forwarded-For + X-Real-IP) to the `GET /api/v1/auth/sessions` read-back.

Verification:
- `cd apps/backend && go test ./internal/auth/httpmw -run '^TestSessionIPRefreshedThroughMiddleware$' -count=1` → PASS (4 sub-cases).
- `cd apps/backend && go test ./internal/auth/httpapi -run '^TestLoginSessionIP' -count=1` → PASS (6 tests).
- `cd apps/backend && go test ./internal/auth/... -count=1` → PASS.
- `go test -race -count=1 ./internal/auth/...` → PASS (race clean).

Files changed: `httpmw/middleware.go` (`c.ClientIP()` + doc-comment fix),
`httpmw/middleware_test.go` (`newTestService` returns the store, `newTestRouter`
variadic trusted proxies clearing gin's trust-all default, new
`TestSessionIPRefreshedThroughMiddleware`), `httpapi/handlers_test.go`
(`loginSessionIP` idempotent read-back transport).

Remaining risks: none. The full-chain refresh is covered by the four sub-cases
and the landed trusted-proxies suite stays deterministic under the new touch
behavior.
