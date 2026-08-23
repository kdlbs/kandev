---
status: active
system: auth
created: 2026-08-17
owners:
  - Kandev
---
# Isolate Kandev cookies between instances on one host Requirements

## Overview

Browsers match cookies by host, domain, and path; **port is not part of cookie scope** (the `Secure` attribute controls transmission over TLS, not scheme partitioning). Every kandev instance bound to the same host (same IP, different ports) therefore shares one cookie jar: a cookie written by the instance the user touched last is sent to every other instance on the next request. `localStorage` and `sessionStorage` are origin-scoped (scheme + host + port) and therefore port-segregated;...

## Requirements

### REQ-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001: Isolate Kandev cookies between instances on one host

**Intent:** Browsers match cookies by host, domain, and path; **port is not part of cookie scope** (the `Secure` attribute controls transmission over TLS, not scheme partitioning). Every kandev instance bound to the same host (same IP, different ports) therefore shares one cookie jar: a cookie written by the instance the user touched last is sent to every other instance on the next request. `localStorage` and `sessionStorage` are origin-scoped (scheme + host + port) and therefore port-segregated;...

#### Acceptance criteria

- **AC-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001.1:** `kandev_session` becomes `kandev_session_<port>` where `<port>` is derived from the request Host (backend-side set and read), honoring `X-Forwarded-Host` when present (same pattern as the existing `X-Forwarded-Proto` TLS detection). A request without a port in its host keeps the plain name (`kandev_session`), so default-port deployments are unchanged. The backend resolves the name through a request-aware service method (`CookieNameForRequest(r)`). The config default for `auth.cookieName` is **empty** (the service keeps the `kandev_session` base fallback internally, so the effective name for default deployments is unchanged): when the configured value is non-empty it is returned verbatim (never suffixed); when empty the default base name is port-suffixed. This lets the resolver distinguish explicit configuration from the default — a seeded non-empty default would defeat suffixing in production. The generic `httpcookie.ScopedName` helper only appends the suffix and is never fed a configured name.
- **AC-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001.2:** `kandev-active-workspace` / `office-active-workspace` become `kandev-active-workspace_<port>` / `office-active-workspace_<port>`. **Both sides derive the suffix from the API origin port** (one canonical port): the SPA takes it from `getBackendConfig().apiBaseUrl` — equal to `window.location.port` in same-origin deployments (Go serves SPA + API on one port) and equal to the backend port in split-origin dev (Vite SPA on the web port, API on the backend port, no Vite proxy — the browser calls the backend directly), which is the same port the backend sees on its request Host; the backend derives it from the request Host (`X-Forwarded-Host` precedence). Deriving from `window.location.port` alone would mismatch the backend in split-origin dev. The unprefixed legacy names remain readable as a fallback (their values are still validated against the workspace list), so a pre-upgrade selection survives; new writes only use the scoped names.
- **AC-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001.3:** The session cookie has **no** legacy fallback: after upgrade each instance requires one re-login. A fallback would re-open the cross-instance bleed for any mixed-version period.
- **AC-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001.4:** **Workspace isolation scope: all-new builds.** The validated legacy fallback means an upgraded instance still reads the unprefixed workspace cookie while **old builds keep writing it**, so during a mixed-version deployment an old instance can overwrite the legacy value a new instance reads (when the id validates against the new instance's workspace list). Workspace-selection isolation therefore applies once **all** instances on the host are upgraded; the legacy fallback exists so pre-upgrade selections survive, and it is safe because resolution validates against the instance's own workspace set (a foreign id falls back gracefully). Sessions are isolated even during mixed versions (no legacy session fallback).
- **AC-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001.5:** **GIVEN** two auth-enabled kandev instances A (port 8080) and B (port 8081) on the same host, **WHEN** the user logs into B, **THEN** A's open session keeps working, and A's next page load shows A's authenticated UI rather than the Sign in page.
- **AC-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001.6:** **GIVEN** the same pair, **WHEN** the user selects a workspace in B, **THEN** A's next boot resolves A's own workspace selection, not B's workspace id.
- **AC-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001.7:** **GIVEN** an instance upgraded from a version that wrote the unprefixed workspace cookie, **WHEN** the instance boots, **THEN** the legacy value is honored when it names one of the instance's own workspaces.
- **AC-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001.8:** **GIVEN** an instance upgraded from a version that wrote the unprefixed session cookie, **WHEN** the user loads the instance, **THEN** the user is asked to sign in once (the legacy token is not accepted), and the new scoped cookie is used afterwards.

## System design

The migrated technical source is split into [part 1](../system-design/fix-multi-instance-cookie-isolation.md).
