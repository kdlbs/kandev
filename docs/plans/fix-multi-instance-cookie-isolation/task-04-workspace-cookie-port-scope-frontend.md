---
id: "04-workspace-cookie-port-scope-frontend"
title: "Port-scope workspace cookies (frontend)"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/auth/requirements/fix-multi-instance-cookie-isolation.md"
---

# Task 04: Port-scope workspace cookies (frontend)

## Acceptance

- `apps/web/lib/routing/route-bootstrap.ts` gains:
  - `scopedCookieName(name: string, port?: string): string` — appends
    `_<port>` when non-empty; the port defaults to the API origin port from
    `getBackendConfig().apiBaseUrl` (NOT `window.location.port`, which
    mismatches in split-origin dev where the SPA runs on the web port and
    the API on the backend port with no Vite proxy).
  - `readScopedCookie(name: string, port?: string)` — reads the scoped name
    first, falls back to the legacy name. The explicit `port` parameter
    keeps both helpers pure and unit-testable without jsdom Location
    stubbing (a same-module function cannot be `vi.spyOn`-stubbed for its
    own callers — the ES-module lexical binding trap).
  - `readActiveWorkspaceCookie` and the office legacy read
    (`readCookie(LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE)`) use them with the
    API-origin default.
- `apps/web/components/app-sidebar/app-sidebar-workspace-navigation.ts`
  `writeWorkspaceCookie` writes the scoped name (same API-origin port) for
  both `kandev-active-workspace` and `office-active-workspace`.
- Server-component and async client readers that hold a CookieStore read the
  scoped name first, legacy fallback via
  `readScopedCookieStoreValue(cookieStore, name, port?)`:
  `apps/web/app/page.tsx`, `apps/web/app/office/layout.tsx`,
  `apps/web/app/settings/layout.tsx`, and
  `apps/web/app/office/lib/get-active-workspace.ts` (the actual helper
  location; not `apps/web/lib/get-active-workspace.ts`).
- `apps/web/src/office-routes.tsx` is a **synchronous client effect** (lines
  ~224-229) and keeps the document-cookie-based `readScopedCookie(name,
  port?)` helpers instead of an async `readCookies()` refactor; both helper
  families share `scopedCookieName` so the derivation is identical. This
  split is explicit: CookieStore readers use the store helper, the sync
  office effect uses `readScopedCookie`.
- **Office precedence unification** (task-03 contract, frontend half):
  `app/office/layout.tsx` and `app/office/lib/get-active-workspace.ts`
  currently resolve the office cookie only, while `office-routes.tsx`
  carries a 5-argument resolver with route precedence. Extract ONE canonical
  frontend resolver with an explicit input contract:
  `resolveOfficeWorkspaceId(items, { routeWorkspaceId, generalWorkspaceId,
  officeWorkspaceId, settingsWorkspaceId })` and a single documented
  precedence — **route → general (when it names an office workspace) →
  office cookie → settings → first** — every candidate validated against the
  office set. Wrappers pass their route override where one exists:
  `getActiveWorkspaceId(urlWorkspaceId?)` passes it as `routeWorkspaceId`
  (its URL-first behavior is already pinned by
  `get-active-workspace.test.ts:58-78`; a valid `/office?workspaceId=O2`
  must beat the cookies); only the no-argument caller
  (`app/office/layout.tsx`) passes `null`. The server layout cannot read
  query params and hydrates from cookies at first paint, so convergence with
  a route override is **client-eventual**: the `office-routes.tsx` bootstrap
  effect re-resolves route-first and hydrates the store. The convergence
  regression asserts the OfficeRoutes bootstrap (route O2 + cookies naming
  O1 → hydrated `activeId` = O2), not the server layout — the layout's
  first-paint value is cookie-driven by design and converges client-side.
- **Structural guard test** (vitest, `fs`-based): (a) negative — no
  workspace-cookie `cookieStore.get(...)` / `readCookie(...)` call exists in
  the five files outside the helpers, with arguments rejected both as
  literals (`"kandev-active-workspace"`, `"office-active-workspace"`) **and
  as the exported constants** (`ACTIVE_WORKSPACE_COOKIE`,
  `LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE` — `app/page.tsx` and
  `app/settings/layout.tsx` read via the constant today); (b) **positive** —
  each of the four CookieStore readers references `readScopedCookieStoreValue`
  and `office-routes.tsx` references `readScopedCookie`, so a reader that
  simply drops its lookup fails the guard. The helper unit tests cover
  scoped-first + legacy fallback.
- Unit tests, failing pre-change:
  - **Reader tests** (beyond the guard, which only proves symbol reference):
    `app/office/lib/get-active-workspace.test.ts` is extended with
    scoped-first (`office-active-workspace_8443`), legacy fallback,
    route-override, and **general-vs-office precedence** cases;
    **`src/office-routes.test.ts` is updated in place** (it currently tests
    the 5-argument resolver including route precedence). **Classification**:
    the resolver **precedence** cases (route > general-if-office > office >
    settings > first, distinct-valid-O1/O2) are **invariant** — the current
    `office-routes.tsx` resolver already implements this precedence and the
    existing `office-routes.test.ts` already covers it; extraction must keep
    them green pre-change. The **failing-before** coverage targets the
    scoped/legacy cookie reads (`scopedCookieName`, `readScopedCookie`,
    `readScopedCookieStoreValue`), the server-reader wrapper wiring
    (`get-active-workspace` + the office layout reading both scoped cookie
    families), and the client convergence path (route override hydrating
    `activeId` — the old office-cookie-only reads are what is actually
    wrong).
  - `route-bootstrap.test.ts`: `scopedCookieName("kandev-active-workspace",
    "8443")` = `kandev-active-workspace_8443`; `readScopedCookie` returns
    the `_8443` value when present and falls back to the legacy name
    otherwise. `readScopedCookieStoreValue` returns the scoped
    `cookieStore` entry first, legacy fallback. A **split-origin
    regression** asserts the default port comes from the API base URL
    (`:38429`) and not from `window.location.port` (`:37429`) — stub
    `getBackendConfig` (separate module, mockable) or pass the port
    explicitly.
  - **Structural guard test** (vitest, `fs`-based): fails on any
    workspace-cookie lookup in the five files outside the helpers — literal
    names **or** the exported constants (`ACTIVE_WORKSPACE_COOKIE`,
    `LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE`) — (negative), and on a file
    that does not reference its mandated helper — the four CookieStore
    readers must import `readScopedCookieStoreValue`, `office-routes.tsx`
    must import `readScopedCookie` (positive).
  - **No-port frontend regression**: an API origin with no explicit port (or
    `port=""`) yields no suffix — `scopedCookieName("kandev-active-workspace",
    "")` stays unprefixed and the write path emits the plain name (the spec's
    single-instance/default-port promise, frontend half; backend session
    no-port is task 02).
  - **Client convergence**: two classes. **(invariant)** OfficeRoutes
    bootstrap with a route override naming O2 and cookies naming O1 hydrates
    `activeId` = O2 — the current route-first resolver already wins, so this
    guards the precedence, not the change. **(failing-before)** a
    **no-route / invalid-route** case with distinct **non-first** scoped
    fixtures: route override absent, scoped general cookie naming O1 (and no
    valid unprefixed cookie) → the bootstrap hydrates O1 (pre-change the
    reader never sees the scoped name → settings/first, so the assertion
    fails pre-change).
  - **OfficeRoutes scoped-office bootstrap regression** (failing-before,
    the scoped **office-family** read — the convergence case above covers
    the general family only): OfficeRoutes bootstrap with scoped general
    `kandev-active-workspace_8443` naming a **kanban** workspace, scoped
    office `office-active-workspace_8443` naming non-first O2, no legacy
    office cookie, no route/settings candidate → hydrated `activeId` = O2.
    Pre-change the effect's direct `readCookie(LEGACY_OFFICE_ACTIVE_
    WORKSPACE_COOKIE)` reads only the unprefixed name (absent) → settings/
    first; fails pre-change. Implement via a focused bootstrap test or a
    narrow exported cookie-candidate reader used by the effect.
  - **Per-file guard argument assertions**: the structural guard also checks
    each CookieStore reader passes the correct cookie name — `app/page.tsx`
    and `app/settings/layout.tsx` must reference `ACTIVE_WORKSPACE_COOKIE`
    (not the legacy office constant) through the helper, and
    `app/office/layout.tsx` + `get-active-workspace.ts` must pass BOTH
    `ACTIVE_WORKSPACE_COOKIE` and `LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE` in
    the documented order (general first); **`office-routes.tsx` must
    reference `readScopedCookie` with the legacy office constant** for its
    office candidate (defense-in-depth against the wrong family/name).
    Text/AST assertions in the guard test, or focused reader tests; a wrong
    constant or a dropped candidate fails the suite.
  - `app-sidebar-workspace-navigation.test.ts` **and
    `app-sidebar-workspace-picker.test.tsx`** (the picker drives the same
    `rememberWorkspaceSelection` path and today asserts unprefixed writes):
    `rememberWorkspaceSelection` writes `kandev-active-workspace_8443=...`
    (and `office-active-workspace_8443=...` for office workspaces) **and
    does not write the unprefixed names** (negative assertion — the legacy
    read fallback stays, but no new legacy writes); the picker fixtures and
    captured cookie-name assertions are updated to the API-port suffix.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/routing/route-bootstrap.test.ts components/app-sidebar/app-sidebar-workspace-navigation.test.ts components/app-sidebar/app-sidebar-workspace-picker.test.tsx app/office/lib/get-active-workspace.test.ts src/office-routes.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/routing/route-bootstrap.ts`
- `apps/web/lib/routing/route-bootstrap.test.ts`
- `apps/web/components/app-sidebar/app-sidebar-workspace-navigation.ts`
- `apps/web/components/app-sidebar/app-sidebar-workspace-navigation.test.ts`
- `apps/web/components/app-sidebar/app-sidebar-workspace-picker.test.tsx`
- `apps/web/app/page.tsx`
- `apps/web/app/office/layout.tsx`
- `apps/web/app/settings/layout.tsx`
- `apps/web/app/office/lib/get-active-workspace.ts`
- `apps/web/app/office/lib/get-active-workspace.test.ts`
- `apps/web/src/office-routes.tsx` (office bootstrap cookie reads)
- `apps/web/src/office-routes.test.ts` (resolver tests updated/moved in place)
- `apps/web/lib/config.ts` (only if the API-origin port needs a small accessor)

## Dependencies

None (independent of the Go helper).

## Parallelism

Parallel-safe with `task-01` (disjoint tree/contract).

## Inputs

- Spec: `What` (workspace cookie scoping — API-origin port on both sides,
  legacy fallback), `Scenarios` (second, third, split-origin).
- Plan: `Frontend`.
- Existing pattern: `writeWorkspaceCookie` is the single client write
  choke point; `readCookie` is the single client read choke point;
  `getBackendConfig()` in `apps/web/lib/config.ts` already resolves the
  API base URL (split-origin dev vs same-origin prod).

## Risks

- Split-origin dev is the trap: any default not derived from the API base
  URL silently degrades to the legacy fallback. The split-origin regression
  test is the guard.
- Do not stub a same-module helper with `vi.spyOn` — the ES-module lexical
  binding is not replaced for its own callers; tests pass the `port`
  parameter explicitly instead.
- Do not scope `kandev_locale` or `layout:*` reads (out of scope); keep the
  change limited to the workspace cookies.
- `office-routes.tsx` reads both cookies through `readActiveWorkspaceCookie`
  / `readCookie`; verify the office bootstrap uses the scoped variants.

## Output contract

Report the scoped read/write behavior, changed files, exact commands and
results, blockers/risks, then mark this task `done` and update `plan.md`.
