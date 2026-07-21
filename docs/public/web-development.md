---
title: "Web Development"
description: "Develop Kandev's Vite/React routes, domain data flow, settings, workbench, state, and responsive UI."
---

# Web Development

`apps/web/` is a React 19 single-page application built by Vite. In production, the Go backend serves its output and remains the API, WebSocket, and authorization boundary.

## Run the application

```bash
make dev
```

Use the combined development supervisor for feature work. It selects available ports, starts Go and Vite, and makes Go proxy the Vite UI. `make dev-web` starts Vite alone on `localhost:37429`; it does not provide a backend. Do not hardcode the auto-selected combined-development ports.

## Trace a route

Startup flows through:

```text
src/main.tsx
  -> src/boot-payload.ts
  -> components/state-provider.tsx
  -> src/app-shell.tsx
  -> src/spa-routes.tsx
```

The client first uses `window.__KANDEV_BOOT_PAYLOAD__`, or fetches `/api/v1/app-state`, then hydrates state before rendering route content. Routing is Kandev's own SPA router in `lib/routing/`, not React Router or Next.js. Files such as `app/tasks/[id]/page.tsx` are components; their paths do not create routes.

Top-level matching lives in `src/spa-routes.tsx`. Settings use `src/settings-routes.tsx`; task detail enters through `src/task-detail-route.tsx`. Add a route to the explicit table and its tests rather than relying on the filesystem.

## Trace domain data

- `lib/api/domains/` owns typed HTTP clients.
- `hooks/domains/` owns feature query/mutation behavior.
- `lib/state/store.ts`, `lib/state/slices/`, and `lib/state/hydration/` own shared Zustand state.
- `lib/ws/client.ts`, `lib/ws/router.ts`, and `lib/ws/handlers/` apply live updates.
- `app/` holds page-level feature components.
- `components/` holds feature and reusable composition.
- `apps/packages/ui/`, `types/`, and `theme/` hold cross-workspace packages.

Use `@kandev/ui` for shared primitives. Add a state slice only when data is genuinely shared or event-driven; route-local server data does not automatically need global state.

The backend remains authoritative. A mutation should use the owning domain client/hook, and state must tolerate duplicates, reconnects, late events, and a route changing during a response. Do not reimplement workflow transitions, executor lifecycle, provider permissions, or Git rules in a component.

When a wire field changes, update the Go DTO/event, TypeScript type, HTTP client, hydration, WebSocket handler, and compatibility tests that consume it. Prefer additive changes when released clients can overlap.

## Add or change settings

Settings pages live under `app/settings/` and `components/settings/`. Route matching is in `src/settings-routes.tsx`. Navigation is assembled under `components/app-sidebar/sections/settings/` and, for general settings, `components/settings/general-nav.ts`.

A setting normally needs:

1. a backend-owned type, validation, scope, and persistence/secret strategy;
2. a typed client in `lib/api/domains/` and domain hook;
3. route and navigation registration;
4. page/components for loading, empty, invalid, saved, error, and missing-dependency states;
5. state/hydration only if other screens consume it;
6. route/unit tests and a Playwright user journey.

Preserve workspace, instance, or global scope in every request and link. Make restart requirements, unavailable dependencies, and experimental status visible.

## Change the task workbench

`/t/:id` resolves through `src/task-detail-route.tsx`, `app/tasks/[id]/kanban-task-shell.tsx`, and `components/task/task-page-content.tsx`. The advanced desktop layout uses `components/task/dockview-desktop-layout.tsx`, `lib/state/dockview-store.ts`, and `lib/state/layout-manager/`. Mobile task layout lives separately under `components/task/mobile/`.

Chat, plan, changes, files, terminal, editor/diff, preview, commit, and pull-request panels share task, session, repository, and executor state. Test a changed action with:

- no repository and multiple repositories;
- preparing, running, stopped, failed, and completed sessions;
- stale or delayed HTTP/WebSocket data;
- a restored dock layout and a narrow mobile route;
- absent optional providers or executor capabilities.

## Security, accessibility, and responsiveness

Frontend checks improve the experience; they are not authorization. The backend must enforce identity, workspace/task scope, and destructive operations. Treat provider text, repository content, Markdown/HTML, URLs, and agent output as untrusted. Keep raw HTML rendering paired with the existing sanitizer and never interpolate shell commands or credentials in the browser.

Use semantic controls and labels, visible focus, complete keyboard interaction, named icon buttons, status text beyond color, and reduced-motion behavior. Verify narrow viewports without overflow or desktop-only hover assumptions.

## Test and review

```bash
make test-web
make typecheck-web
make lint-web
make test-e2e
```

Vitest tests are colocated with source. Playwright fixtures and specs are under `apps/web/e2e/`. Repository policy requires every `apps/web/` change to add or update a Playwright scenario; seed isolated test data and never use a developer's normal database or workspace.

Before review, confirm loading/empty/error/reconnect states, keyboard names, desktop and mobile layouts, Go/TypeScript wire compatibility, focused unit tests, truthful Playwright coverage, and updated public screenshots or terminology.

Related: [Architecture](architecture.md), [Testing](testing.md), and [Developer tools](developer-tools.md).
