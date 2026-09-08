---
id: "02-auth-session-ip-tests"
title: "Auth session-IP behavioral tests"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/auth/requirements/trusted-proxies.md"
---

# Task 02: Auth session-IP behavioral tests

Lock the observable contract: the session IP stored by login (and recorded for
setup / invite acceptance) honors `X-Forwarded-For` only when the TCP peer is a
trusted proxy, and falls back to the peer otherwise.

## Acceptance

- With trusted proxies containing the peer (e.g. `10.0.0.0/8` for peer
  `10.0.0.5`), a login request carrying `X-Forwarded-For: 203.0.113.7` stores
  session IP `203.0.113.7` (read back via `GET /api/v1/auth/sessions`).
- With no trusted proxies (the default) or a CIDR not containing the peer
  (e.g. `192.168.0.0/16`), the same request stores session IP `10.0.0.5`.
- With a trusted peer and only `X-Real-IP: 203.0.113.7`, the stored session
  IP is `203.0.113.7`.
- With a trusted peer, `X-Forwarded-For: not-an-ip` and
  `X-Real-IP: 203.0.113.7`, the stored session IP is `203.0.113.7`.
- With a trusted peer and neither forwarded header, the stored session IP is
  the peer `10.0.0.5`.
- The auth test fixture mirrors production: `gin.New()` defaults to trusting
  all proxies, so `newAPIFixture` must explicitly clear them
  (`router.SetTrustedProxies(nil)`) unless the test passes a trusted list.

## Files likely touched

- `apps/backend/internal/auth/httpapi/handlers_test.go` (fixture +
  new tests; existing tests keep passing unchanged)

## Dependencies

None. The fixture-level tests configure the router directly
(`SetTrustedProxies` via `newAPIFixture`), so they exercise gin's behavior at
the auth surface without Task 01's wiring; Task 01 covers env parsing and the
production wiring in `backendapp`.

## Parallelism

`sequential` by default; parallel-safe candidate with Task 01 (disjoint
files: `internal/auth/httpapi/*` vs `internal/backendapp/*`) only when the
user authorizes subagents.

## Inputs

- Spec **Scenarios** (all nine; the six login tests exercise scenarios 1, 2,
  3, 7, 8, 9 as mapped in plan.md Tests; scenarios 4-6 are covered there and
  in Task 01) and acceptance a/b.
- `apps/backend/internal/auth/httpapi/handlers_test.go`:
  `newAPIFixture`, `apiClient.do`, `TestFullLifecycle` (setup + invite
  flow) as the request-shape reference.
- `apps/backend/internal/auth/httpapi/handlers.go:85-106` (`setup`, `login`)
  and `handlers_admin.go:120-126` (`acceptInvite`) pass `c.ClientIP()` into
  the service; session IP is exposed by `listSessions` as the `ip` field.
- The test client needs a request seam: `apiClient.do` currently builds the
  request and serves it immediately, so add a variadic request mutator
  (e.g. `do(method, path, body, mutate ...func(*http.Request))`) to set
  `RemoteAddr` and forwarded headers.
- Account bootstrap: `newAPIFixture(t, true)` starts in setup mode (no admin
  yet), so tests must first `POST /api/v1/auth/setup` (as `TestFullLifecycle`
  does) before login has an identity. After the mutated login, `GET
  /api/v1/auth/sessions` lists both the setup session and the login session;
  select the `current: true` row (the login cookie is current) and assert its
  `ip` field.
- `httptest.NewRequest` sets `RemoteAddr` to `192.0.2.1:1234` by default;
  tests must override `req.RemoteAddr` and set the `X-Forwarded-For` header
  explicitly.

## TDD sequence

1. Add the request seam to `apiClient.do`, write the no-trusted-proxies
   (peer-IP) test (bootstrap via `POST /api/v1/auth/setup`, mutated login,
   then select the `current: true` session), and run it RED against the
   current fixture: gin's trust-all default would honor the header, so the
   peer-IP assertion fails.
2. Extend `newAPIFixture(t, authEnabled, trustedProxies ...string)` to call
   `router.SetTrustedProxies(trustedProxies)` (variadic; no args → `nil`,
   matching production), add the remaining cases (trusted peer → header IP;
   not-matching CIDR → peer IP; `X-Real-IP` only; malformed XFF + valid
   `X-Real-IP`; neither header → peer IP), and run GREEN.
3. Confirm the full auth HTTP suite stays green: `go test ./internal/auth/...`.

## Verification

```bash
cd apps/backend && go test ./internal/auth/httpapi -run '^TestLoginSessionIP' -count=1 && go test ./internal/auth/... -count=1
```

The through-proxy manual smoke is the plan-level integration gate (see
plan.md "Manual integration smoke"), executed after Tasks 01-02 land, because
it needs the production wiring plus the running backend; record the concrete
blocker in `## Results` if the environment cannot run a proxied instance.

## Risks

- If the fixture keeps gin's trust-all default, the "unset → header ignored"
  assertion is meaningless; the fixture change is mandatory, not optional.
- gin reads `X-Real-IP` as a secondary header; tests must set only
  `X-Forwarded-For` so the assertion isolates that path.

## Output contract

Report the RED failure, GREEN result, exact files changed, fixture-change
evidence, remaining risks, and synchronized task/plan status. Record every
exact command and outcome in `## Results`.

## Results

Extended `apps/backend/internal/auth/httpapi/handlers_test.go`: `apiClient.do`
now takes a variadic request mutator; `newAPIFixture` accepts a variadic
trusted-proxy list and calls `SetTrustedProxies` (nil by default, mirroring
production); added `loginSessionIP` (setup bootstrap → mutated login →
`current: true` session) and 6 `TestLoginSessionIP*` tests.

- RED: `cd apps/backend && go test ./internal/auth/httpapi -run '^TestLoginSessionIPNoTrustedProxiesIgnoresForwardedFor$' -count=1` — `session IP = "203.0.113.7", want peer 10.0.0.5` (gin's trust-all fixture default honored the header).
- GREEN: `go test ./internal/auth/httpapi -run '^TestLoginSessionIP' -count=1` — ok (6 tests: no-trusted-proxies, trusted peer, untrusted CIDR, X-Real-IP only, malformed XFF + valid X-Real-IP, neither header).
- Full auth suite: `go test ./internal/auth/... -count=1` — ok (auth, auth/httpapi, auth/httpmw, auth/store).
- Manual smoke is the plan-level integration gate; blocked in this harness (no proxied deployment).
