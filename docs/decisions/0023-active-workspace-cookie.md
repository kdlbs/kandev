# 0023: Active workspace cookie for boot state

**Status:** accepted
**Date:** 2026-06-18
**Area:** backend, frontend

## Context

The Vite migration moved first-paint data loading from Next server components to Go boot payloads. Active workspace selection still came from several places: URL params, user settings, an Office-named cookie, and kanban localStorage. Go cannot read localStorage while serving HTML, so a hard reload could hydrate a different workspace than the browser sidebar remembered.

## Decision

Kandev will use `kandev-active-workspace` as the general browser cookie for the current active workspace. The Go boot-state builders and SPA route bootstraps read it after explicit URL workspace params and before user settings. The legacy `office-active-workspace` cookie remains a read fallback and is still written for Office compatibility during migration.

The cookie stores only the workspace ID. Broader preferences and filters should not move into cookies by default: durable user settings stay in backend user/workspace settings, shareable route state stays in URL params, and purely local view state may stay in localStorage. Add another cookie only when Go must know that value before serving the SPA shell.

## Consequences

Hard reloads, production boot payloads, and Vite dev app-state fetches can resolve the same active workspace. The cookie is sent on normal HTTP requests, so keeping it limited to a small non-sensitive ID avoids unnecessary request bloat and avoids leaking external integration data into the boot path.

Vite dev uses a separate web port, so boot-state fetches must include credentials and the Go CORS middleware must echo the request origin when allowing credentials. Production remains same-origin because Go serves both the SPA and API.

## Alternatives Considered

1. **Keep kanban localStorage only.** Rejected because Go cannot read it for first-paint boot state.
2. **Use only user settings.** Rejected because Office intentionally does not overwrite kanban's shared `workspace_id`, and quick workspace switches should not necessarily mutate durable settings.
3. **Move all UI settings to cookies.** Rejected because cookies are sent on every request and should stay limited to values the server needs before serving HTML.
