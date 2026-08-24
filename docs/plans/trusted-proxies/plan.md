---
spec: docs/specs/auth/requirements/trusted-proxies.md
created: 2026-08-15
status: building
---

# Implementation Plan: Trusted Proxies for X-Forwarded-For

## Overview

Make the backend honor `X-Forwarded-For` only when the request's TCP peer is
one of the proxies listed in `KANDEV_TRUSTED_PROXIES` (comma-separated IPs /
CIDRs), while keeping the current secure default (no trusted proxies) when the
variable is unset, empty, or contains an unparsable entry. The change is one
env-var read plus a `SetTrustedProxies` call at the single router construction
site in `buildHTTPServer`; every `ClientIP()` consumer (login / setup / invite-accept / plugin-SSO
session IPs and the login rate-limiter key) picks up the behavior with no
per-callsite changes; `RequestLogger` never reads `ClientIP()` and stays
untouched.

Order: Tasks 01 (parser + wiring with backendapp unit tests) and 02 (auth
HTTP behavioral tests that pin the observable session-IP contract; they
configure the router at the fixture level and are independent of the wiring)
are parallel-safe Wave-1 candidates with disjoint files, sequential by default
absent user authorization; Task 03 (public configuration docs) lands as Wave
2 after them.

## Backend

### Env parsing and router wiring (`apps/backend/internal/backendapp/trustedproxies.go`, new)

- `const trustedProxiesEnv = "KANDEV_TRUSTED_PROXIES"`.
- `func resolveTrustedProxies(raw string) (trusted []string, invalid []string)`:
  an entirely blank raw value is unset (`nil`, no invalid). Otherwise split on
  `,`, trim each entry, and validate each with `net.ParseCIDR` when it
  contains `/`, else `net.ParseIP`; a blank component inside a nonblank value
  is an invalid entry (named `<empty>`), because empty is not an IP or CIDR.
  Any invalid entry makes the whole list fail closed: `trusted` is `nil` and
  `invalid` names the bad values. (Reasons: gin v1.9.1 `SetTrustedProxies`
  errors on an unparsable entry, and the task requires never partially
  trusting a list.)
- `func configureTrustedProxies(router *gin.Engine, log *logger.Logger)`:
  reads `os.Getenv(trustedProxiesEnv)`, calls `resolveTrustedProxies`, logs a
  `Warn` naming the invalid values when any exist (and forces `trusted =
  nil`), then calls `router.SetTrustedProxies(trusted)` with the existing
  `log.Warn` fallback on error. `nil` matches the current production call.
- Env-reading follows the repo pattern of direct `os.Getenv` at the call site
  (compare `KANDEV_WEB_DIST_DIR` in `helpers.go`, `KANDEV_CONSOLE_LOG_LEVEL` in
  `main.go`). No new config framework, no YAML key.

### Router construction site (`apps/backend/internal/backendapp/main.go`)

- In `buildHTTPServer`, replace the comment block at lines 1924-1930 and the
  `router.SetTrustedProxies(nil)` call with the new explanatory comment
  (documents `KANDEV_TRUSTED_PROXIES`, the spoofing tradeoff for a directly
  reachable backend, and the no-trusted-proxies default) and a call to
  `configureTrustedProxies(router, log)`.
- No other router construction sites exist for the main HTTP surface; the
  agentctl control server is loopback-only and out of scope.

## Frontend

No UI changes. `ClientIP()` feeds the session IP shown in
Settings > Account > Security, which renders the already-stored value.

## Tests

- **Parse rules** (`apps/backend/internal/backendapp/trustedproxies_test.go`):
  table test over `resolveTrustedProxies` covering entirely blank raw value,
  single CIDR, multiple CIDRs with whitespace, bare IPv4/IPv6, IPv6 CIDR,
  unparsable entry, mixed valid+invalid (must return `trusted == nil`), and
  blank components inside a nonblank value (leading/trailing/doubled commas)
  must return `trusted == nil` with `<empty>` in `invalid`.
- **Env wiring + warning** (same file): `configureTrustedProxies` with
  `t.Setenv`; behavioral assertion via a `gin.New()` router serving a handler
  that returns `c.ClientIP()`: trusted peer + `X-Forwarded-For` header
  resolves the header value; unset env and not-matching CIDR return the TCP
  peer; invalid env logs a warning naming the bad value (zap observer pattern:
  `zaptest/observer` + `logger.NewFromZap`, used across the repo) and behaves
  like unset.
- **Session IP contract** (`apps/backend/internal/auth/httpapi/handlers_test.go`):
  add a request-mutation seam to the test client (variadic mutator on
  `apiClient.do`) so tests can set `RemoteAddr` and forwarded headers, and
  extend `newAPIFixture` to clear trusted proxies by default (`router.
  SetTrustedProxies(nil)`, mirroring production) and accept an optional
  trusted-proxy list. Named tests (anchored `-run '^TestLoginSessionIP'`),
  login with `req.RemoteAddr = "10.0.0.5:1234"`, then read the stored session
  IP via `GET /api/v1/auth/sessions`:
  - trusted `10.0.0.0/8` + `X-Forwarded-For: 203.0.113.7` → session IP `203.0.113.7`;
  - no trusted proxies + the same header → session IP `10.0.0.5`;
  - trusted `192.168.0.0/16` (peer not in list) + the same header → session IP `10.0.0.5`;
  - trusted peer + `X-Real-IP: 203.0.113.7` only → session IP `203.0.113.7`;
  - trusted peer + `X-Forwarded-For: not-an-ip` + valid `X-Real-IP` → session IP from `X-Real-IP`;
  - trusted peer + neither header → session IP `10.0.0.5`.
  In listed order these exercise spec scenarios 2 (trusted peer + XFF), 1
  (unset), 3 (untrusted CIDR), 8 (X-Real-IP only), 9 (malformed XFF + valid
  X-Real-IP), 7 (neither header). Spec scenario 4 (invalid env → warning +
  nil trust) is covered by Task 01's backendapp tests; scenarios 5-6
  (spoofing) are the trust mechanics of scenarios 2/3 from an attacker
  framing and need no separate test: a peer inside a trusted range is
  believed, a peer outside every range is ignored. The pre-fixture RED case
  is the no-trusted-proxies assertion (gin's trust-all default would honor
  the header); the trusted cases are GREEN at the fixture level regardless of
  Task 01, so Task 02 does not depend on it. Acceptance c (invalid value) is
  covered by the backendapp warning + nil-fallback test combined with the
  no-trusted-proxies session test.
- No E2E: no user-facing flow changes (the Settings page renders a stored
  value; the behavior difference is only which IP is stored).
- **Manual integration smoke** (post-implementation gate, after Tasks 01-02
  land): run the backend with `KANDEV_TRUSTED_PROXIES=<proxy-cidr>`, log in
  through the proxy, and confirm Settings > Account > Security shows the real
  client IP. If the environment cannot run a proxied instance, record the
  concrete blocker in the plan's Verification Results instead of dropping the
  check.

## Verification Results

- Task 01 parser + wiring: `cd apps/backend && go test ./internal/backendapp -run '^Test(ResolveTrustedProxies|ConfigureTrustedProxies)' -count=1` — RED (build failed: undefined symbols) then GREEN (13 subtests + 4 behavioral tests). Regression `go test ./internal/backendapp/... -count=1` — ok. `go build ./...` — ok.
- Task 02 session-IP contract: `cd apps/backend && go test ./internal/auth/httpapi -run '^TestLoginSessionIP' -count=1` — RED (unset case stored forwarded IP under gin's trust-all default) then GREEN (6 tests). Full auth suite `go test ./internal/auth/... -count=1` — ok.
- Task 03 docs: `node scripts/validate-public-docs.mjs && git diff --check` — "Validated 41 published docs pages." + clean diff.
- Combined gate: `cd apps/backend && go build ./... && go test ./internal/auth/... ./internal/backendapp/... -count=1` — all ok (auth, auth/httpapi, auth/httpmw, auth/store, backendapp, backendapp/ownershiplock).
- Manual integration smoke: BLOCKED — no proxied kandev deployment available in this harness (needs a running backend + reverse proxy + browser login). The observable contract is covered by the session-IP tests above; run the smoke in the Test step against a real proxy deployment.

## Implementation Waves And Parallel Candidates

Wave 1 (parallel candidates — user authorization required; files are disjoint:
`internal/backendapp/*` vs `internal/auth/httpapi/*`):

- [x] [Task 01: Backend trusted-proxies parser and wiring](task-01-backend-trusted-proxies.md)
- [x] [Task 02: Auth session-IP behavioral tests](task-02-auth-session-ip-tests.md)

Wave 2:

- [x] [Task 03: Configuration docs](task-03-configuration-docs.md)

Task 02 exercises the router at the fixture level and does not depend on Task
01's code; Task 03 is disjoint from both. All sequential by default per repo
convention; no subagent execution is authorized by this plan.

## Risks and out of scope

- gin v1.9.1 `New()` trusts all proxies (`0.0.0.0/0`, `::/0`) by default, so
  both production and the auth test fixture must keep an explicit
  `SetTrustedProxies(nil)` default; the fixture change is part of Task 02.
- Setting the variable while the backend is directly reachable allows
  `X-Forwarded-For` spoofing and defeats the ClientIP-keyed login rate
  limiter; this is the documented tradeoff, not a defect.
- `X-Forwarded-Proto`, WebSocket `remote_addr` logging, and `RequestLogger`
  fields stay untouched.
