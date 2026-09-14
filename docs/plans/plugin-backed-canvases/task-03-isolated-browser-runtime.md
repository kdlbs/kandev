---
id: "03-isolated-browser-runtime"
title: "Isolated browser runtime"
status: done
wave: 3
depends_on:
  - "02-plugin-web-app-package-foundation"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-ISOLATED-WEB-APPS-001
  - REQ-PLUGINS-ISOLATED-WEB-APPS-003
  - REQ-PLUGINS-ISOLATED-WEB-APPS-007
  - REQ-PLUGINS-ISOLATED-WEB-APPS-008
  - REQ-PLUGINS-ISOLATED-WEB-APPS-009
  - REQ-PLUGINS-ISOLATED-WEB-APPS-010
acceptance_criteria:
  - AC-PLUGINS-ISOLATED-WEB-APPS-001.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-001.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-003.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-003.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-003.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-003.4
  - AC-PLUGINS-ISOLATED-WEB-APPS-003.5
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.4
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.5
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.6
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.7
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.8
  - AC-PLUGINS-ISOLATED-WEB-APPS-008.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-009.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-009.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-009.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-010.3
system_design:
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
---

# Task 03: Isolated browser runtime

## Summary

Serve immutable releases through revocable capabilities. Enforce the opaque
sandbox for iframe and direct navigation in web and Tauri hosts.

## In scope

- Issue and revalidate user-bound and release-bound capability URLs.
- Serve safe package MIME types without ambient session authorization.
- Build CSP and network policy from normalized grants.
- Add response-level sandbox, referrer, MIME, cache, CORP, and framing headers.
- Add the shared frontend web-application frame.
- Add Tauri `frame-src` and runtime `frame-ancestors` support.
- Add direct-navigation, cookie, CORS, Tauri, and policy regression tests.

## Out of scope

- Kandev data, state, events, canvas metadata, and navigation.

## Acceptance

- Direct and framed runtime documents remain opaque and cannot reach host state.
- Revoked, stale, foreign, expired, or unavailable capabilities fail closed.
- Web and desktop hosts load the runtime while other parents cannot frame it.

## Verification

```bash
cd apps/backend && go test ./internal/plugins/webapp/... ./internal/plugins/...
cd apps && pnpm --filter @kandev/web test -- components/plugins lib/plugins
cd apps && pnpm --filter @kandev/desktop typecheck
```

## Files likely touched

- `apps/backend/internal/plugins/webapp/runtime.go`
- `apps/backend/internal/plugins/webapp/tokens.go`
- `apps/backend/internal/plugins/webapp/policy.go`
- `apps/backend/internal/plugins/handlers.go`
- `apps/web/components/plugins/web-app-frame.tsx`
- `apps/web/components/plugins/web-app-frame.test.tsx`
- `apps/desktop/src-tauri/tauri.conf.json`
- desktop runtime smoke fixtures

## Dependencies

- Task 02 provides validated releases, instances, grants, and availability.

## Risks

- One sandbox token can collapse the opaque-origin boundary.
- A CORP or ancestor value can block one supported host.
- A redirect or referrer can expose a runtime capability.

## Parallelism

`sequential`

## Inputs

- Browser host, runtime token, browser security, compatibility, and failure
  sections.
- Current port-proxy capability and desktop CSP patterns.

## Results

- Added digest-only, user and release-bound runtime capabilities with expiry,
  renewal, revocation, and stale-binding validation.
- Added immutable artifact serving with safe MIME selection, no-store and
  isolation headers, opaque-origin CORS handling, response CSP sandboxing,
  and normalized HTTPS network policy.
- Added the responsive `WebAppFrame` host with the required opaque iframe
  sandbox and mobile safe-area behavior, plus the desktop loopback `frame-src`
  policy and Tauri ancestor support.
- Verification passed for the backend runtime packages, the new frontend
  component, i18n catalogs, and desktop TypeScript. The broad plugin frontend
  suite reached 258 passing tests but reported one existing Monaco command
  registration race as an unhandled error.
