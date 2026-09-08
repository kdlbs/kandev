---
spec: docs/specs/auth/requirements/session-ip-refresh.md
created: 2026-08-19
status: complete
---

# Implementation Plan: Session IP Refresh

## Overview

Refresh a browser session's recorded IP on the existing throttled touch path
instead of pinning it at login. The service's `ResolveSessionToken` gains a
client-IP parameter; on the touch interval it passes the request IP to
`Store.TouchSession`, which refreshes the `ip` column only when the value is
non-empty. The single production caller — the global auth middleware
(`httpmw.ResolveRequest`) — supplies `c.ClientIP()`, so every
COOKIE-authenticated HTTP and WebSocket request benefits. PAT-authenticated
requests (including `?token=` WS upgrades, which resolve via
`AuthPolicy.ResolveToken`, never `ResolveRequest`) carry no session IP and
are unchanged. The HTTP surface and the UI are unchanged.

Order: Task 01 lands the store + service behavior (signature changes and
their tests, including mechanical call-site updates in the `auth` package);
Task 02 wires `c.ClientIP()` through the middleware and proves the full chain
with an integration test.

## Backend

### Store (`apps/backend/internal/auth/store/store.go`)

- `TouchSession(ctx, id, lastSeen, expires)` → `TouchSession(ctx, id,
  lastSeen, expires, ip string)`. Single UPDATE keeps refreshing timestamps
  and conditionally the IP:

  ```sql
  UPDATE auth_sessions
  SET last_seen_at = ?, expires_at = ?,
      ip = CASE WHEN ? = '' THEN ip ELSE ? END
  WHERE id = ?
  ```

  Contract: timestamps always update; `ip` refreshes the stored value only
  when non-empty (empty never clobbers). The service decides *whether* a
  refresh is wanted by passing the request IP only when it differs from the
  stored value; the `CASE` is the store's defense against empty clobbering.

### Service (`apps/backend/internal/auth/service_credentials.go`)

- `ResolveSessionToken(ctx, token)` → `ResolveSessionToken(ctx, token, ip
  string)`. Inside the existing `now.Sub(sess.LastSeenAt) > sessionTouchInterval`
  gate:

  ```go
  refreshIP := ""
  if ip != "" && ip != sess.IP {
      refreshIP = ip
  }
  _ = s.store.TouchSession(ctx, sess.ID, now, now.Add(s.SessionTTL()), refreshIP)
  ```

  Same-IP and empty-IP requests pass `""` (timestamps still update; IP
  preserved). Different non-empty IPs ride the same one-minute throttle —
  no per-request writes. Note: the `ip != sess.IP` comparison is a
  write-avoidance optimization, not an observable contract — an
  always-write service (passing `ip` unconditionally) is behaviorally
  equivalent because the store `CASE` guard and the touch throttle still
  bound writes; the planned tests deliberately do not discriminate it
  (only a test-only counting wrapper over the store could observe the `ip`
  argument, and the same-IP case correctly pins the observable — stored IP
  unchanged while timestamps advance, since a literal "no IP write"
  assertion would contradict the sliding-expiry touch).

### Middleware (`apps/backend/internal/auth/httpmw/middleware.go`)

- Task 01 performs the mechanical signature update
  (`svc.ResolveSessionToken(ctx, cookie)` → `svc.ResolveSessionToken(ctx,
  cookie, "")`) so `internal/httpmw` keeps compiling and the full
  `internal/auth` suite stays green at the Task 01 boundary.
- Task 02 replaces `""` with `c.ClientIP()`:
  `svc.ResolveSessionToken(ctx, cookie, c.ClientIP())`. This is the only
  production call site. WebSocket upgrades refresh IPs through this same
  global middleware on the `/ws` route (`ResolveRequest` runs before the
  deferred-path check, and the gateway's `requireConnectionAuth` only
  consumes the already-resolved identity); the gateway itself never calls
  `ResolveRequest` — PAT `?token=` upgrades go through
  `AuthPolicy.ResolveToken` instead. The same refresh path holds for every
  other deferred path — `/port-proxy/`, `/terminal/`, `/lsp/`, `/vscode/`,
  `/mcp`, office-with-bearer, and the SPA-shell branch: cookie-authenticated
  requests to them resolve and refresh through the global middleware BEFORE
  the deferred-path check, while capability-authenticated proxy requests
  (no `kandev_session` cookie) and `?token=` PAT upgrades correctly never
  touch. Do NOT gate the
  `ResolveRequest` call on `isDeferredPath` — a "skip resolve for deferred
  paths" optimization would silently kill WS + proxy-subtree refresh while
  leaving every OTHER planned test green — task-02 sub-case (d) covers
  this exact regression. Task 02 also corrects the stale doc
  comment block on `httpmw/middleware.go:57-58` ("Shared with the WS
  gateway's upgrade-time check"): no WS caller of `ResolveRequest` exists,
  so the comment misleads implementers into wiring the upgrade path
  separately (now in Task 02's Acceptance and TDD step 2 so it cannot be
  silently dropped).

### Call-site updates (mechanical, same packages)

- `apps/backend/internal/auth/service_test.go` and `service_sso_test.go`:
  every `ResolveSessionToken(ctx, token)` call gains a trailing `""`.
- `apps/backend/internal/auth/store/store_test.go`:
  `TouchSession(ctx, id, lastSeen, expires)` gains a trailing `""` in
  `TestSessionLifecycle`.

## Frontend

No UI changes. `GET /api/v1/auth/sessions` keeps returning the `ip` field;
`Settings > Account > Security` renders the refreshed value.

## Tests

- **Store IP semantics** (`store_test.go`, new `TestTouchSessionIPRefresh`):
  create a session with IP `1.1.1.1`; touch with `2.2.2.2` → read back
  `2.2.2.2` with updated timestamps; touch with `""` → IP stays `2.2.2.2`;
  session created with an EMPTY stored IP, touch with `2.2.2.2` → backfilled
  (spec scenario 4).
- **Service refresh behavior** (`service_test.go`, new
  `TestResolveSessionTokenRefreshesChangedIP`): the store is reachable from
  package-auth tests as `f.svc.store` (no fixture change); tests age a
  session by creating it directly via `GenerateSessionToken()` (unqualified
  — `service_test.go` is package `auth`) + `f.svc.store.CreateSession`
  with `LastSeenAt: now.Add(-2*time.Hour)` (≥2
  minutes in the past — a seed under 60s keeps the strict `>` gate closed
  and fails the aged cases spuriously as a fixture defect). Cases: different
  IP on touch → stored IP updated (read back via `svc.ListSessions`); empty
  stored IP + non-empty request IP → backfilled; same IP → unchanged AND
  `last_seen_at` ADVANCED past the aged value; empty request IP → unchanged
  AND `last_seen_at` ADVANCED — the last_seen-advance pins discriminate a
  refresh-gated touch that stalls sliding expiry (task-01); DIFFERENT IP
  within the touch interval → IP unchanged AND `LastSeenAt` still equals
  the (truncated-to-second) future seed — the within-interval resolve
  passes a different IP (`2.2.2.2`) so the case exercises spec scenario 5's
  observable directly, and always-touch variants self-reveal (a touch
  without the gate either writes the different IP, caught by the IP
  assertion, or bumps `last_seen_at` from the future seed, caught by the
  pin); the required aged-extension (same different IP after aging →
  updated) completes spec scenario 5's post-interval transition. The within-interval service case creates its session
  with `LastSeenAt: now.Truncate(time.Second).Add(10*sessionTouchInterval)`
  — package `auth` can name the constant; the truncation matches the
  `Equal()` convention — so the gate (strict `now.Sub(LastSeenAt) >
  sessionTouchInterval` on a seed 10 intervals in the future) stays closed
  for ~11 minutes of wall-clock regardless of host slowness (the gate
  opens at `D > 660s − f`, where `f ∈ [0,1s)` is the fractional second the
  truncation dropped from the seed, i.e. between 659s and 660s) while
  preserving the
  always-touch discriminator. (The middleware-level sub-cases in Task 02
  cannot name the constant from package `httpmw` and keep the "≥2 minutes
  in the past" recipe.)
- **Middleware integration** (`middleware_test.go`, new
  `TestSessionIPRefreshedThroughMiddleware`): full chain gin → middleware →
  service → store, in three transport sub-cases plus a deferred-path
  sub-case (see task-02): (a) cleared
  proxies + RemoteAddr-only; (b) trusted peer `10.0.0.0/8` +
  `X-Forwarded-For` (a RemoteAddr-wired middleware fails); (c) cleared
  proxies + `X-Forwarded-For` (a raw-header-wired middleware fails); (d) a
  deferred-path request (`GET /ws`, 404-after-resolution is fine) with
  cookie + RemoteAddr → still refreshes (closes the
  skip-resolve-for-deferred-paths regression) —
  (b)+(c) pin the spec's "same `ClientIP()` value that records the IP at
  login, including `KANDEV_TRUSTED_PROXIES` resolution"), plus a real-route
  `GET /api/v1/auth/sessions` read-back in (a). `newTestService` gains an
  out-return for the store; each sub-case creates an aged session with IP
  `1.1.1.1` on a FRESH fixture and requests `/api/v1/probe` with the
  session cookie, asserting `svc.ListSessions` reports the expected IP.
  The `RemoteAddr` differs per sub-case: `2.2.2.2:1234` for (a) (no
  forwarded headers, so `ClientIP()` = RemoteAddr → expect `2.2.2.2`),
  `10.0.0.5:1234` (inside the trusted `10.0.0.0/8`) with
  `X-Forwarded-For: 2.2.2.2` AND `X-Real-IP: 9.9.9.9` for (b) → expect
  `2.2.2.2` (the third header pins XFF-over-X-Real-IP precedence, so
  header-order and X-Real-IP-wired bugs fail deterministically), and
  `10.0.0.5:1234` with `X-Forwarded-For: 2.2.2.2` on the cleared-proxy
  router for (c) → expect `10.0.0.5`. A sub-case (b) that reuses RemoteAddr
  `2.2.2.2` would pass vacuously (untrusted peer, header ignored) and let a
  RemoteAddr-wired middleware through — the exact bug (b) exists to catch.
- Spec scenario coverage: scenario 1 → service different-IP case +
  middleware integration; 2 → service same-IP case; 3 → service empty-IP
  case; 4 → empty-STORED-IP backfill (store + service cases); 5 → service
  within-interval case; 6 → the login-time recording
  regression net is the trusted-proxies suite `TestLoginSessionIP*` in
  `apps/backend/internal/auth/httpapi/handlers_test.go` (`loginSessionIP`
  helper). Task 02 step 3 hardens the helper so its read-back
  `GET /api/v1/auth/sessions` re-applies the login request's FULL transport
  mutator closure (RemoteAddr + `X-Forwarded-For` + `X-Real-IP`), making the
  new touch-based refresh idempotent on that read in all six cases — without
  it, a >60s gap between login and the read would overwrite the asserted IP
  (with the GET's default `httptest` RemoteAddr, or with the fixture's
  RemoteAddr where the stored IP came from a forwarded header), adding a
  timing sensitivity the suite did not have before. Name the suite
  explicitly so it is not mistaken for pass-through coverage of the
  refresh.
- No E2E: no user-facing flow changes and a browser cannot change its own
  client IP mid-session (see spec "Out of scope").

## E2E Tests

None — the change is backend-only with no UI delta; see the Tests section and
spec "Out of scope" for the justification.

## Verification Results

Task 01 and Task 02 landed (see each task's `## Results` for per-step RED/GREEN
evidence). Commands and outcomes:

```bash
# Task 01 — store + service
cd apps/backend && go test ./internal/auth/store -run '^TestTouchSessionIPRefresh' -count=1   # PASS
cd apps/backend && go test ./internal/auth -run '^TestResolveSessionTokenRefreshesChangedIP' -count=1   # PASS, 5 sub-cases
cd apps/backend && go test ./internal/auth/... -count=1   # PASS (auth, httpapi, httpmw, store)

# Task 02 — middleware wiring
cd apps/backend && go test ./internal/auth/httpmw -run '^TestSessionIPRefreshedThroughMiddleware$' -count=1   # PASS, 4 sub-cases
cd apps/backend && go test ./internal/auth/httpapi -run '^TestLoginSessionIP' -count=1   # PASS, 6 tests
cd apps/backend && go test ./internal/auth/... -count=1   # PASS
go test -race -count=1 ./internal/auth/...   # PASS

# Repo gates
make fmt                                        # PASS, no unrelated churn
make -C apps/backend vet                        # PASS
make typecheck                                  # PASS (after generating apps/web/generated/{changelog,release-notes}.json, build-time fixtures absent in a fresh worktree)
make -C apps/backend lint                       # PASS, 0 issues (golangci-lint; run with GOCACHE/GOTMPDIR relocated off quota'd ~/.kandev/cache and ~/.cache)
make lint-web lint-harness lint-architecture    # PASS
```

Environment notes (all verified pre-existing on a clean tree via `git stash`):
- Full backend `go test ./...` initially hit `disk quota exceeded` compile
  failures because the sandbox's `~/.kandev/cache` and `~/.cache` carry quotas;
  relocating `GOCACHE`/`GOTMPDIR` to `~/{gocache,gotmp}` fixed the compile
  failures. Remaining backend failures are fixture/environment-bound:
  `internal/agentctl/server/{api,process,config}` (agent binaries, workspace
  fixtures, missing `/tmp/cargo/env`), `internal/launcher` (systemd/launchd
  unavailable), `internal/plugins/runtime` (install-state fixture) — identical
  failure sets with and without this change.
- Web vitest: 1501/1506 files pass; `lib/http-git-server.test.ts` requires a
  Docker daemon (absent here); the other failures were 5s/10s timeouts under
  load and pass on re-run. Zero frontend files changed by this feature.
- `make test` at root therefore reports the pre-existing environmental
  failures above; no auth-package test fails.

## Implementation Waves And Parallel Candidates

```text
Wave 1:
- [x] [task-01-store-service-refresh](task-01-store-service-refresh.md)

Wave 2:
- [x] [task-02-middleware-wiring](task-02-middleware-wiring.md)
```

Task 02 depends on Task 01 (the new `ResolveSessionToken` signature), so the
waves are sequential. Both are small and stay in the primary conversation;
no subagent execution is authorized by this plan.

## Open Questions

None.
