---
id: "01-shared-port-cookie-helper"
title: "Shared port-scoped cookie name helper"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/auth/requirements/fix-multi-instance-cookie-isolation.md"
---

# Task 01: Shared port-scoped cookie name helper

## Acceptance

- `apps/backend/internal/common/httpcookie` exposes
  `PortSuffix(r *http.Request) string` and
  `ScopedName(r *http.Request, base string) string`:
  - `ScopedName(r, "kandev_session")` returns `kandev_session_8443` when the
    request host carries port 8443, and `kandev_session` when it does not.
  - `X-Forwarded-Host` (first value) wins over `Host`; a bracketed IPv6 host
    (`[::1]:8080`) yields `_8080`; an empty/malformed host yields no suffix.
- Unit tests cover every branch: Host port, no port, X-Forwarded-Host
  precedence, comma-separated and whitespace-padded `X-Forwarded-Host`
  (first value only), IPv6 bracket and zone-id forms (`[fe80::1%25eth0]:8080`
  style), malformed/default-port hosts, nil request (no suffix), empty base.
  **Port validation**: the suffix requires a decimal port in 1..65535 —
  nonnumeric service names (`example.com:http`) and out-of-range values
  (`example.com:99999`) yield no suffix. **Empty base**: `ScopedName(r, "")`
  returns the empty string (a suffix is meaningless without a base name) —
  the empty-base test asserts exactly that. A **conflicting Host/XFH test**
  pins the precedence contract: Host `public.example:8443` +
  `X-Forwarded-Host: public.example:9443` yields `_9443` (XFH wins; a
  misconfigured proxy is the deployment's responsibility, not the
  resolver's).
- The helper is a pure suffixing function: it never knows about config, so a
  caller must not feed it a configured custom cookie name.
- `gofmt` clean; `make lint` in `apps/backend` passes.

## Verification

```bash
cd apps/backend && go test ./internal/common/httpcookie/... && go vet ./internal/common/httpcookie/...
```

## Files likely touched

- `apps/backend/internal/common/httpcookie/httpcookie.go` (new)
- `apps/backend/internal/common/httpcookie/httpcookie_test.go` (new)

## Dependencies

None. This is the shared contract both backend scoping tasks import.

## Parallelism

Parallel-safe with `task-04` (frontend, disjoint tree).

## Inputs

- Spec: `What` (port-scoped names, X-Forwarded-Host precedence) and
  `Failure modes` (proxy, IPv6).
- Existing pattern: `apps/backend/internal/auth/httpapi/cookie.go`
  `requestIsTLS` (X-Forwarded-Proto handling).

## Risks

- Keeping suffix derivation in one place is the point; do not inline it in
  callers. The helper must not parse `X-Forwarded-Host` values that contain
  multiple hosts (take the first, trimmed). The custom `auth.cookieName`
  verbatim rule lives in the auth service (`CookieNameForRequest`), never in
  this helper.

## Output contract

Report the helper API, changed files, exact commands and results,
blockers/risks, then mark this task `done` and update `plan.md`.
