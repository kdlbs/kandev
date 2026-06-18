# 0024: Go-fronted Vite dev mode

**Status:** accepted
**Date:** 2026-06-18
**Area:** backend, frontend, cli

## Context

After migrating production to a Go-served Vite SPA, `make dev` still opened the Vite dev server directly. That kept React Fast Refresh, but it made development materially different from production: browser requests were cross-origin, route reloads did not naturally exercise Go shell rendering, and boot-payload behavior could differ from `make start`.

## Decision

Development mode uses the Go backend as the browser entrypoint. The CLI still starts Vite as an internal frontend transform and HMR server, but it prints and opens the backend URL. The backend's `KANDEV_WEB_INTERNAL_URL` path now uses `webapp.DevHandler`: HTML navigation requests fetch Vite's current `index.html`, inject the Go boot payload, and return the shell from Go; Vite module, static asset, and HMR requests are proxied through to the internal Vite server.

Browser-facing Vite env vars that force cross-origin API calls, such as `VITE_KANDEV_API_PORT`, are unset in dev. Frontend code should use the same-origin runtime config injected by the Go boot payload, matching production.

## Consequences

Full reloads in dev now exercise the same Go route classification and boot-payload path as production while retaining Vite's fast TypeScript/React transform loop and HMR. CORS becomes less central to normal local development because the app URL is same-origin.

Vite remains a development dependency. Replacing Vite's transform, React Fast Refresh, worker handling, CSS pipeline, and module graph with Go-native tooling is out of scope. If a production-like no-HMR loop is needed later, it can be added as a separate `vite build --watch` plus Go static-serving mode.

## Alternatives Considered

1. **Keep opening Vite directly.** Rejected because it preserves cross-origin browser behavior and misses Go boot-payload rendering on document requests.
2. **Replace Vite entirely in dev.** Rejected because rebuilding Vite's transform and HMR feature set in Go would add substantial custom tooling with little product value.
3. **Use `vite build --watch` and full-page reloads as default dev.** Rejected as the default because it is more production-like but loses React Fast Refresh. It remains a reasonable optional mode.

No feature spec update is required; this is an internal development-runtime decision, not a user-facing product capability.
