# 0022: Embedded Vite Assets

**Status:** accepted
**Date:** 2026-06-17
**Area:** backend, frontend, cli

## Context

ADR-0021 moved Kandev toward a Go-served Vite SPA with boot state and no production Node.js web runtime. The first implementation still required a built `apps/web/dist` directory beside the backend binary in local `start` and release bundles, with `KANDEV_WEB_DIST_DIR` pointing the backend at those files. That kept frontend serving in Go, but it left production packaging dependent on an external asset directory.

## Decision

Production builds embed the Vite output into the backend binary. The root build runs `build-web`, syncs `apps/web/dist` into `apps/backend/internal/webapp/embedded/generated`, and then compiles `cmd/kandev` with those assets embedded via `go:embed`. The backend still accepts `KANDEV_WEB_DIST_DIR` as an explicit override for local debugging, previews, and tests.

Release bundles no longer require a sibling `web/` directory for runtime. The CLI starts the backend without setting `KANDEV_WEB_DIST_DIR`, so the normal production path serves the embedded assets.

## Consequences

Production runtime packaging is simpler: the backend binary contains the SPA shell and static assets, while the CLI remains only a launcher. Build ordering now matters: the web build must be synced before compiling the backend for release-quality binaries. Backend-only builds remain possible because the embed package includes a minimal fallback shell, but those binaries are not release-quality frontend builds.

Tests and preview flows can continue to set `KANDEV_WEB_DIST_DIR` when they need to serve a mutable filesystem dist.

## Alternatives Considered

1. **Keep filesystem-only assets.** Rejected because it preserves an avoidable runtime packaging dependency.
2. **Move Vite output permanently under `apps/backend`.** Rejected because it couples frontend build output to backend source layout and increases generated-file churn.
3. **Embed only for release builds with build tags.** Rejected as unnecessary complexity; an override env var is enough for non-release workflows.
