---
id: "01-backend-trusted-proxies"
title: "Backend trusted-proxies parser and wiring"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/auth/requirements/trusted-proxies.md"
---

# Task 01: Backend trusted-proxies parser and wiring

Parse `KANDEV_TRUSTED_PROXIES` at the router construction site and pass the
result to `router.SetTrustedProxies`, keeping the no-trusted-proxies default
and a fail-closed warning path for invalid entries.

## Acceptance

- `resolveTrustedProxies` returns `(nil, nil)` for an entirely blank raw
  value; the valid CIDRs/IPs for a fully valid comma-separated list; and
  `(nil, [bad…])` when any entry is unparsable, including a blank component
  inside a nonblank value (named `<empty>`): never a partially trusted list,
  never silent trust from a trailing/doubled comma.
- `configureTrustedProxies` reads the env var, applies valid lists to the
  router, and for invalid entries logs a startup WARNING naming the bad
  value(s) and applies `SetTrustedProxies(nil)`.
- `buildHTTPServer` calls `configureTrustedProxies` in place of the current
  hardcoded `SetTrustedProxies(nil)`, and the replaced comment documents the
  env var, the spoofing tradeoff (a directly reachable backend with the var
  set can have `X-Forwarded-For` spoofed, which also defeats the
  ClientIP-keyed login rate limiter), and the default (no trusted proxies).
- Behavior: with the env var set to a CIDR containing the peer, a request
  carrying `X-Forwarded-For` resolves the header value via `c.ClientIP()`;
  with the var unset, or set to a CIDR not containing the peer, or invalid,
  `c.ClientIP()` returns the TCP peer.

## Files likely touched

- `apps/backend/internal/backendapp/trustedproxies.go` (new)
- `apps/backend/internal/backendapp/trustedproxies_test.go` (new)
- `apps/backend/internal/backendapp/main.go` (comment + call at ~line 1924)

## Dependencies

None.

## Parallelism

`sequential` by default. Files are disjoint from Task 02's auth fixture tests;
no subagent execution is authorized by this plan.

## Inputs

- Spec sections **What**, **Failure modes**, **Scenarios**.
- `apps/backend/internal/backendapp/main.go` `buildHTTPServer` (router
  construction at lines 1924-1933).
- gin v1.9.1 semantics: `SetTrustedProxies(nil)` disables header trust;
  `SetTrustedProxies` returns an error on unparsable entries, so pre-validate
  with `net.ParseIP` / `net.ParseCIDR`.
- Env pattern: direct `os.Getenv` at the call site (e.g. `KANDEV_WEB_DIST_DIR`
  in `helpers.go`). `net` and `strings` are already imported in `main.go`.
- Log-warning assertion pattern: `go.uber.org/zap/zaptest/observer` +
  `logger.NewFromZap(zap.New(core))` (used across `internal/backendapp`).

## TDD sequence

1. Write `trustedproxies_test.go` first: table test for `resolveTrustedProxies`
   (entirely blank raw, single CIDR, multi with whitespace, bare IPv4/IPv6,
   IPv6 CIDR, invalid, mixed valid+invalid, blank components from
   leading/trailing/doubled commas → invalid `<empty>`) and behavioral
   `configureTrustedProxies` tests (env set/valid, unset, not-matching CIDR,
   invalid with warning) using a `gin.New()` router serving
   `c.String(http.StatusOK, c.ClientIP())` with `httptest.NewRequest` +
   `RemoteAddr` + `X-Forwarded-For`. Run and confirm RED.
2. Implement `trustedproxies.go`.
3. Update `main.go` comment + call. Run the targeted tests GREEN, then
   `go build ./...` and `go test ./internal/backendapp/...` for regressions.

## Verification

```bash
cd apps/backend && go test ./internal/backendapp -run '^Test(ResolveTrustedProxies|ConfigureTrustedProxies)' -count=1 && go test ./internal/backendapp/... -count=1 && go build ./...
```

## Risks

- Forgetting that gin trusts all proxies by default would silently defeat the
  secure default; the unset-env test asserting the TCP peer guards this.
- Passing an unvalidated list to `SetTrustedProxies` returns an error and
  leaves ambiguous state; the parser must never forward invalid entries.

## Output contract

Report the RED failure, GREEN result, exact files changed, the warning-log
evidence, remaining risks, and synchronized task/plan status. Record every
exact command and outcome in `## Results`.

## Results

Implemented `apps/backend/internal/backendapp/trustedproxies.go`
(`trustedProxiesEnv`, `emptyTrustedProxyEntry`, `resolveTrustedProxies`,
`configureTrustedProxies`), replaced the hardcoded `SetTrustedProxies(nil)`
block in `buildHTTPServer` (`main.go`) with the documented
`configureTrustedProxies(router, log)` call, and added
`trustedproxies_test.go`.

- RED: `cd apps/backend && go test ./internal/backendapp -run '^Test(ResolveTrustedProxies|ConfigureTrustedProxies)' -count=1` — build failed with undefined `resolveTrustedProxies`, `configureTrustedProxies`, `trustedProxiesEnv`, `emptyTrustedProxyEntry`.
- GREEN: the same command after implementation — ok (13 parse subtests + 4 behavioral tests). One test-only fix: the warning assertion inspected `field.String` on a `zap.Strings` array field, which does not render; switched to `fmt.Sprint(field.Interface)` (the warning itself was emitted correctly, with `invalid_entries: [not-a-cidr]`).
- Regression: `go test ./internal/backendapp/... -count=1` — ok (backendapp 11.4s, ownershiplock).
- Build: `go build ./...` — ok.
- Security: fail-closed rule enforced in `resolveTrustedProxies` (any invalid entry, including blank components from trailing/doubled commas, yields `trusted == nil` and names the bad values); the warning is logged before `SetTrustedProxies(nil)`.
