---
spec: docs/specs/ui/requirements/mobile-task-navigation.md
related_specs:
  - docs/specs/system-page/requirements/system-page.md
decision: docs/decisions/2026-07-27-spa-failure-containment-and-deployment-recovery.md
created: 2026-07-27
status: complete
---

# Implementation Plan: SPA Blank-Screen Resilience

## Overview

Prevent known task and Settings failures from emptying the Kandev SPA. Add
visible route loading and two-tier error containment, recover once from stale
Vite chunks, reload the document after a confirmed service self-update, remove
render-created Settings thenables, and stabilize mobile repository selectors.

This is a behavior-preserving reliability fix, so it does not create a new
feature spec. The mobile task spec owns the multi-repository regression, the
System-page spec owns the post-update document reload, and the cross-cutting
application invariants are recorded in the linked ADR. No backend API,
persistence, or public documentation changes are required.

## Success Criteria

- Settings and Office chunk loading shows an accessible progress surface on
  desktop and mobile.
- A route render/import failure preserves application chrome and offers hard
  reload; a shell/provider failure renders a dependency-minimal full-page
  recovery action.
- The first Vite preload failure in a 60-second tab window causes one guarded
  reload. A persistent failure stops reloading and renders recovery UI.
- A service self-update reloads the document exactly once, and only after the
  target backend version is confirmed.
- Every SPA-mounted dynamic Settings page receives synchronous identifier
  props, and `/settings/executor/new` resolves to the create route.
- The mobile multi-repository picker tolerates missing keyed repository and
  session collections without an unstable external-store snapshot.
- Focused unit/component tests, desktop and mobile production E2E, and the full
  repository verification gates pass.

## Frontend Design

### Failure containment and visible loading

- Add a reusable application error boundary with distinct root and route
  fallbacks.
- Mount the root boundary outside `StateProvider`.
- Mount the route boundary inside `AppShell`; reset it after an error only when
  pathname/search changes.
- Replace null Settings and Office Suspense fallbacks with a shared accessible
  route loader.
- Preserve narrower plugin boundaries and avoid exposing raw errors in UI.

### Deployment and update recovery

- Install a Vite preload-error listener before React bootstrap.
- Use a verified `sessionStorage` marker with a 60-second TTL to permit one
  automatic reload and let repeated failures reach the error boundary.
- Skip automatic recovery when storage cannot prove the loop guard.
- Replace the service update's metadata-only completion callback with a stable
  injectable document reload callback.
- Guard overlapping update polls so confirmed completion fires once.

### Render determinism

- Pass decoded identifiers directly from `settings-routes.tsx` to all nine
  SPA-mounted dynamic Settings pages.
- Remove `use(params)`, async client wrappers, and render-created
  `Promise.resolve(params)` calls from those routes.
- Reserve `/settings/executor/new` before the dynamic executor-id matcher.
- Use typed module-level empty sentinels for the mobile repository and session
  selectors.

## Mobile Design Contract

- **Desktop outcome and mobile entry:** both retain the current app shell and
  route navigation; mobile enters Settings through the existing Home menu and
  opens repositories through the existing task repository pill.
- **Nearest exemplars:** the current task-route loading state and
  `PluginErrorBoundary` establish loading and containment behavior.
- **Hierarchy and primary action:** loading occupies the route content area;
  route recovery keeps the shell available and presents a touch-sized Reload
  action. A root failure presents only the fatal recovery surface.
- **Presentation and rationale:** use inline status/alert surfaces, not a
  dialog or drawer, because the blocked route cannot support a temporary
  overlay reliably.
- **Touch/geometry:** recovery actions are at least 44px high, remain within
  the narrow viewport and safe area, and introduce no document horizontal
  overflow.
- **Shared logic:** desktop and mobile share boundaries, preload guard,
  Settings props, and selector invariants; only responsive shell chrome differs.
- **Mobile proof:** production Playwright covers delayed/failed Settings chunks
  and a multi-repository picker with missing keyed hydration.

## Implementation Waves

Wave 1 (parallel-safe candidates with non-overlapping ownership):

- [x] [Task 01 — Contain SPA failures and show route loading](task-01-failure-containment.md)
- [x] [Task 02 — Make Settings route props synchronous](task-02-settings-route-determinism.md)
- [x] [Task 03 — Stabilize mobile repository selectors](task-03-mobile-selector-stability.md)
- [x] [Task 05 — Reload after confirmed self-update](task-05-self-update-reload.md)

Wave 2:

- [x] [Task 04 — Guard stale Vite preload recovery](task-04-preload-recovery.md)
  — depends on Task 01 because both own `main.tsx`

Wave 3:

- [x] [Task 06 — Prove production desktop/mobile resilience](task-06-browser-regressions.md)
  — depends on Tasks 01–05

Wave 4:

- [x] [Task 07 — Run integrated repository gates](task-07-integrated-verification.md)
  — depends on Tasks 01–06

Tasks in a parallel-safe wave still have durable, disjoint ownership. The
primary session updates each task and this plan after accepting its results.

## Risks

- Preventing a repeated `vite:preloadError` would swallow Vite's throw and
  leave Suspense pending; only the first guarded reload attempt may call
  `preventDefault()`.
- Clearing the preload marker at initial mount can recreate a loop before the
  lazy route succeeds; use expiration instead.
- Error-boundary reset logic must not remount healthy routes or erase normal
  application state.
- Self-update polling intervals can overlap; completion and reload must be
  idempotent.
- Dynamic Settings matchers can shadow static routes; table-driven route tests
  must cover all migrated identifiers and reserved paths.
- A mocked Zustand hook cannot reproduce snapshot identity behavior; the
  component regression must use the real store provider.

## Out of Scope

- Settings bootstrap cancellation, stale-response reconciliation, or redesign
  of every page-level `return null` branch.
- A router rewrite, service worker, or retaining multiple embedded asset
  generations.
- Converting Office's cached parameter promises.
- Backend cache headers, APIs, schemas, or persistence.
- Unrelated Settings information architecture or visual redesign.

## Verification

Targeted tests:

```bash
cd apps/web
pnpm test -- src/app-error-boundary.test.tsx src/spa-routes.loading.test.tsx
pnpm test -- src/settings-routes.test.ts
pnpm test -- components/task/mobile/mobile-repos-section.test.tsx
pnpm test -- src/vite-preload-recovery.test.ts
pnpm test -- hooks/domains/system/use-self-update.test.ts components/settings/system/updates-card.test.tsx
```

Production browser regressions:

```bash
cd apps/web
pnpm e2e:run --project chromium -- tests/layout/spa-resilience.spec.ts tests/system/updates-page.spec.ts --workers=1
pnpm e2e:run --project mobile-chrome -- tests/layout/mobile-spa-resilience.spec.ts --workers=1
```

Full repository gates, in order:

```bash
make fmt
make typecheck
make test
make lint
```

Final integrated result: formatting, metadata generation, typecheck, tests, and
lint passed. The full suite covered 206 backend test packages, 925 web test
files with 7,063 passing tests and 4 skips, and 30 CLI test files with 280
passing tests. The local verification environment used `umask 022` with
`TMPDIR` and `GOTMPDIR` set to `/var/tmp` so Go temporary fixtures satisfied the
repository-parent permission invariant.
