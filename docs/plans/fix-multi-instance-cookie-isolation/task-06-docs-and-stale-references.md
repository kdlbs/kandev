---
id: "06-docs-and-stale-references"
title: "Deployment note and stale cookie-name references"
status: done
wave: 3
depends_on: ["02-session-cookie-port-scope-backend", "04-workspace-cookie-port-scope-frontend"]
plan: "plan.md"
spec: "../../specs/auth/requirements/fix-multi-instance-cookie-isolation.md"
---

# Task 06: Deployment note and stale cookie-name references

## Acceptance

- `docs/public/authentication.md` gains an operator note: multiple
  auth-enabled instances on one host (same IP, different ports) are isolated
  per port via port-scoped cookie names. Reverse proxies: the CORS/WS origin
  gate compares the browser `Origin` hostname with the request `Host` and
  ignores `X-Forwarded-Host`, so the proxy MUST preserve the browser
  **hostname**; it may either preserve the full `Host` (host:port) or rewrite
  only the port and forward a correct `X-Forwarded-Host` (the resolver takes
  the public port from XFH) — rewriting a non-loopback hostname 403s before
  auth (loopback-alias rewrites such as `localhost` → `127.0.0.1` pass the
  existing origin gate and are unchanged).
  **Session-cookie migration** (scoped to auth sessions; workspace cookies
  are separate): old auth-enabled builds conflict with other old builds via
  the shared unprefixed `kandev_session`; an upgraded instance ignores the
  legacy session token and requires one re-login (guaranteed on rollback and
  re-upgrade because the new build never reads the unprefixed session
  cookie); workspace selections keep their validated legacy read fallback, so
  a pre-upgrade selection survives. Custom `auth.cookieName` values disable
  automatic port isolation and must be unique per cookie host. Instances on
  the same host at default ports over different schemes (HTTP :80 + HTTPS
  :443) keep the plain names and are not isolated.
- `docs/public/configuration.md` documents `auth.cookieName` with its
  canonical environment override `KANDEV_AUTH_COOKIE_NAME` (explicit
  `BindEnv`; the automatic camelCase mapping would expose the undocumented
  `KANDEV_AUTH_COOKIENAME`), plus config.yaml and precedence
  (env over file over default).
- Stale references that pin the literal `kandev_session` cookie name are
  updated to describe "the session cookie (name derived from the request
  host)" or "base name `kandev_session`; effective name request-host-derived":
  - `apps/web/e2e/helpers/auth.ts` (doc comment, ~lines 6-8)
  - `apps/web/components/app-sidebar/app-sidebar-workspace-navigation.ts`
    (comment ~lines 30-34: "office paths read the legacy cookie... kept in
    step" — legacy names are now read-only fallback, writes are scoped)
  - `apps/web/app/office/layout.tsx` (comment ~lines 154-157: the office
    cookie "is set by the setup wizard and workspace rail" — now writes the
    port-scoped name)
  - `docs/specs/plugins/requirements/plugins.md` (~lines 223-234, external-login section)
  - `docs/decisions/0050-plugin-external-auth-capability.md`
  - `docs/decisions/2026-07-24-opt-in-authentication.md`
  - `apps/backend/AGENTS.md`
  - `apps/backend/internal/backendapp/auth.go` (plugin SSO bridge doc
    comment, ~lines 21-24)
  - `apps/backend/internal/auth/service_sso.go` (comment)
  - `apps/backend/internal/plugins/auth_login.go` (comment)
  - `apps/backend/internal/auth/store/models.go` (comment)
- **No-op verification targets** (already amended by this package; keep in
  the audit grep, do NOT edit): `docs/specs/auth/requirements/auth.md` (already states
  base name + request-host-derived effective names) and
  `docs/decisions/2026-08-15-office-mode-follows-active-workspace.md`
  (already states the legacy office cookie is read-only fallback and writes
  are port-scoped).
- No behavioral code change in this task; `gofmt`/lint still clean (comments
  only).

## Verification

```bash
rg -n "kandev_session" docs/specs/auth/requirements/auth.md docs/specs/plugins/requirements/plugins.md docs/decisions/0050-plugin-external-auth-capability.md docs/decisions/2026-07-24-opt-in-authentication.md apps/backend/AGENTS.md apps/backend/internal/auth apps/backend/internal/plugins apps/backend/internal/backendapp/auth.go apps/web/e2e/helpers/auth.ts apps/web/components/app-sidebar/app-sidebar-workspace-navigation.ts apps/web/app/office/layout.tsx
rg -n "office-active-workspace" docs/decisions/0023-active-workspace-cookie.md docs/decisions/2026-08-15-office-mode-follows-active-workspace.md docs/decisions/2026-07-24-opt-in-authentication.md
# every path listed in the stale-reference acceptance must appear above; remaining hits must be intentional (e.g. the legacy-cookie no-fallback note in the fix spec)
cd apps/backend && go build ./...
```

## Files likely touched

- `docs/public/authentication.md`
- `docs/public/configuration.md` (new `auth.cookieName` row + env name)
- `docs/specs/plugins/requirements/plugins.md`
- `apps/web/e2e/helpers/auth.ts` (doc comment)
- `apps/web/components/app-sidebar/app-sidebar-workspace-navigation.ts`
- `apps/web/app/office/layout.tsx`
- `docs/decisions/0050-plugin-external-auth-capability.md`
- `docs/decisions/2026-07-24-opt-in-authentication.md`
- `apps/backend/AGENTS.md`
- `apps/backend/internal/backendapp/auth.go`
- `apps/backend/internal/auth/service_sso.go`
- `apps/backend/internal/plugins/auth_login.go`
- `apps/backend/internal/auth/store/models.go`

Audit-only (do NOT edit; already amended by this package, keep in the
verification grep): `docs/specs/auth/requirements/auth.md`,
`docs/decisions/2026-08-15-office-mode-follows-active-workspace.md`.

## Dependencies

The behavior contract from tasks 02 and 04 (docs describe shipped behavior).

## Parallelism

Wave 3, parallel-safe with `task-05` (docs vs e2e specs; disjoint).

## Inputs

- Spec: `Failure modes` (proxy, mixed-version), `Out of scope`.
- Plan: `Docs`.
- Existing pattern: `docs/public/plugins-authoring.md` already describes the
  request-host-derived name; mirror that wording.

## Risks

- Some hits are intentional (the fix spec's legacy-fallback note, the auth
  spec's port-scoped bullet); do not rewrite the fix package itself. Only
  prose that implies a fixed literal name is in scope.
- Comments in `service_sso.go` / `auth_login.go` sit next to the SSO bridge
  code changed in task 02; coordinate wording with that task's diff.

## Output contract

Report the deployment note placement, the updated references, the remaining
intentional `kandev_session` hits, changed files, exact commands and results,
blockers/risks, then mark this task `done` and update `plan.md`.
