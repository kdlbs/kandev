---
title: "Web Development"
description: "Develop Kandev's Vite and React web client, state, routes, settings, workbench surfaces, and responsive UI."
---

# Web Development

The web application lives in `apps/web/`. It is a Vite-built React 19 single-page application served by the Go backend in production.

## Run the web client

```bash
make dev
```

Use the combined command for normal feature work so API and WebSocket behavior are present. To isolate the client:

```bash
make dev-web
```

The development command chooses available ports and prints the URLs. Do not hardcode a port in tests or documentation unless the relevant command guarantees it.

## Code layout

- `apps/web/app/` contains route/page-level features.
- `apps/web/components/` contains reusable and feature components.
- `apps/web/lib/api/` contains HTTP clients organized by backend domain.
- `apps/web/lib/state/` contains Zustand state and slices.
- `apps/web/lib/routing/` contains client routing and bootstrap helpers.
- `apps/web/e2e/` contains Playwright fixtures, seed helpers, page objects, and specs.
- `apps/web/src/` contains the application entry, shell, and route dispatch.

Follow an existing nearby feature from API client through state to UI before introducing a new data-fetching or routing pattern.

## Backend state is authoritative

Use typed domain clients for mutations and reconcile the result into state. WebSocket events keep connected screens current, but the client must tolerate reconnects, duplicates, and events arriving while a route is changing.

Do not duplicate workflow transition, executor lifecycle, provider permission, or Git rules in the component layer. The UI can explain why an action is unavailable; the backend must enforce it.

## Settings routes

Settings pages are assembled through `apps/web/src/settings-routes.tsx` and page/components under `apps/web/app/settings/` and `apps/web/components/settings/`. Workspace integrations and automations require an active workspace and should preserve that scope in links and API requests.

When adding a setting, cover initial bootstrap, loading, empty, error, saved, and permission/dependency states. Make restart requirements or experimental status visible.

## Workbench UI

Task detail combines chat, documents, file editor, terminal, changes, preview, and pull-request context. These surfaces share task/session/repository selection, so test a new action with:

- no-repository tasks;
- multi-repository tasks;
- preparing, running, stopped, failed, and completed sessions;
- desktop and mobile navigation;
- delayed WebSocket or API responses.

Keep controls compact and state-aware. Kandev's product principles in `PRODUCT.md` require orientation before controls, density without clutter, familiar developer affordances, keyboard access, visible focus, and reduced-motion support.

## Accessibility and responsive behavior

- Use semantic buttons, links, labels, tabs, dialogs, and headings.
- Keep icon-only controls named for assistive technology and tooltips.
- Preserve visible focus and complete keyboard interaction.
- Do not encode status only with color.
- Respect reduced motion.
- Verify narrow mobile viewports without horizontal overflow or desktop-only assumptions.

## Tests

```bash
make test-web
make typecheck-web
make lint-web
make test-e2e
```

Component/unit tests use Vitest. User-visible changes under `apps/web/` require an updated Playwright E2E scenario. Use established E2E seeding APIs and isolated test data rather than an operator's local database.

## Review checklist

- Uses the existing domain API and state path.
- Covers loading, empty, error, and stale/reconnect behavior.
- Works for every relevant task/session/repository state.
- Keyboard and screen-reader names are correct.
- Desktop and mobile layouts are manually verified.
- Unit tests and a truthful E2E scenario cover the behavior.
- Public docs and screenshots use current terminology and UI.

Related: [Architecture](architecture.md), [Testing](testing.md), and [Developer tools](developer-tools.md).
