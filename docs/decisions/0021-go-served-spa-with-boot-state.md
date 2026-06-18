# 0021: Go-served SPA with boot state

**Status:** accepted
**Date:** 2026-06-15
**Area:** backend, frontend, cli

## Context

Kandev currently ships a Go backend plus a Next.js standalone web server. The CLI starts both processes in production, and the backend reverse-proxies browser routes to the Next server. Next server components also fetch request-time data and hydrate the Zustand store, so removing Next cannot regress first paint into loading spinners.

## Decision

Kandev will migrate the web runtime to a Vite-built React SPA served by the Go backend. Production will not run a Node.js web server. The Go backend will serve embedded frontend assets, route unknown browser paths to the SPA shell, and inject a JSON boot payload containing runtime config, route metadata, and `Partial<AppState>` initial state.

ADR-0022 specifies the packaging detail for those frontend assets: release-quality production builds embed the Vite output into the backend binary, with `KANDEV_WEB_DIST_DIR` retained as an explicit override for non-release workflows.

ADR-0023 specifies the active-workspace persistence rule used by boot state: only the current workspace ID is stored in a server-readable cookie, while broader settings stay in backend settings, URL params, or localStorage.

ADR-0024 specifies the development-mode counterpart: `make dev` still runs Vite internally for transforms and HMR, but browser document requests enter through Go so boot payload behavior matches production.

The boot-state builder will live on the Go side and use backend services directly where practical. The SPA will initialize `StateProvider` from the injected payload, and client-side route transitions that require preloaded data will fetch the same payload shape from a backend app-state endpoint before rendering the destination view.

Full React SSR from Go is explicitly out of scope. The server-rendered surface is the HTML shell and serialized data contract; React renders in the browser.

Operational migration guidance for in-progress PRs lives in [`../nextjs-spa-migration.md`](../nextjs-spa-migration.md).

## Consequences

The production runtime becomes simpler: one Go web process serves HTTP APIs, WebSockets, static assets, SPA fallback, and boot data. Release artifacts no longer need Next.js standalone output or a Node process for the web UI.

The migration must preserve the existing first-paint hydration contract. Some TypeScript SSR mapping logic will need a Go equivalent, so golden tests and narrow mappers are required to avoid drift. Feature gates, redirects, not-found behavior, and active-workspace cookie precedence must move from Next server components into Go boot-state builders.

Development still uses Node-based tooling for Vite, TypeScript, linting, tests, and package management. This decision removes the production Node web runtime first; removing all Node tooling is a separate effort.

## Alternatives Considered

1. **Keep Next.js and optimize packaging.** This preserves existing behavior but keeps the production Node runtime dependency that this migration is meant to remove.
2. **Full React SSR from Go.** Rejected because it would require embedding or supervising a JavaScript runtime, or maintaining a second renderer, which defeats the runtime simplification.
3. **Pure client-side SPA fetches after mount.** Rejected as the target architecture because it would regress first paint for routes that currently SSR-fetch and hydrate Zustand.
