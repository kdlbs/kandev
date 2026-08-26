# 0023: Active workspace cookie for boot state

**Status:** accepted
**Date:** 2026-06-18
**Area:** backend, frontend

## Context

The Vite migration moved first-paint data loading from Next server components to Go boot payloads. Active workspace selection still came from several places: URL params, user settings, an Office-named cookie, and kanban localStorage. Go cannot read localStorage while serving HTML, so a hard reload could hydrate a different workspace than the browser sidebar remembered.

## Decision

Kandev will use `kandev-active-workspace` as the general browser cookie for the current active workspace. Generic boot paths **that accept an explicit workspace URL param** (Home, Tasks, integrations) read it after those params and before user settings; **Settings resolves cookie/settings-first** (no query-param candidate, matching both the backend `settingsWorkspaceID` and the frontend settings bootstrap). Office boot paths are intentionally **cookie-first**: `officeWorkspaces` (server) and the client `OfficeRoutes` in `src/office-routes.tsx` resolve from cookies/settings without a query-param candidate, while the `OfficeRoutes` bootstrap effect honors an explicit `?workspaceId=` (the ADR 2026-08-15 exception) and converge the store client-side. The legacy `office-active-workspace` cookie remains a read fallback **for the office boot paths only**; it is no longer written (see the 2026-08-17 amendments — new writes use only the port-scoped names).

**Amended 2026-08-17 (family separation):** cookie families are read
separately. Generic boot paths (Home, Settings, Tasks, integration) read
**only the general family** — `kandev-active-workspace` / the scoped
`kandev-active-workspace_<port>` — and resolve settings/first when only an
office cookie exists. Only office boot paths (`officeWorkspaces` and the
client `OfficeRoutes` in `src/office-routes.tsx`) read the **office family**
(`office-active-workspace` / the scoped
`office-active-workspace_<port>`), as one candidate among general, office,
and settings. The backend generic reader previously consulted the legacy
office name; that candidate is removed to match the frontend generic reader.
See `docs/specs/auth/requirements/fix-multi-instance-cookie-isolation.md`.

**Amended 2026-08-17:** the cookie names are port-scoped when the instance is
served on a non-default port (`kandev-active-workspace_<port>` /
`office-active-workspace_<port>`). Both sides derive the suffix from the API
origin port: the request host server-side (`X-Forwarded-Host` precedence)
and the API base URL (`getBackendConfig().apiBaseUrl`) client-side —
`window.location.port` only equals it in same-origin deployments. Cookies
ignore port in their scope, so multiple instances on one host (same IP,
different ports) otherwise overwrite each other's active-workspace selection.
The legacy unprefixed names remain readable as a validated fallback; new
writes use only the scoped names. See
`docs/specs/auth/requirements/fix-multi-instance-cookie-isolation.md`.

The cookie stores only the workspace ID. Broader preferences and filters should not move into cookies by default: durable user settings stay in backend user/workspace settings, shareable route state stays in URL params, and purely local view state may stay in localStorage. Add another cookie only when Go must know that value before serving the SPA shell.

## Consequences

Hard reloads, production boot payloads, and Vite dev app-state fetches can resolve the same active workspace. The cookie is sent on normal HTTP requests, so keeping it limited to a small non-sensitive ID avoids unnecessary request bloat and avoids leaking external integration data into the boot path.

Vite dev uses a separate web port, so boot-state fetches must include credentials and the Go CORS middleware must echo the request origin when allowing credentials. Production remains same-origin because Go serves both the SPA and API.

## Alternatives Considered

1. **Keep kanban localStorage only.** Rejected because Go cannot read it for first-paint boot state.
2. **Use only user settings.** Rejected because Office intentionally does not overwrite kanban's shared `workspace_id`, and quick workspace switches should not necessarily mutate durable settings.
3. **Move all UI settings to cookies.** Rejected because cookies are sent on every request and should stay limited to values the server needs before serving HTML.
