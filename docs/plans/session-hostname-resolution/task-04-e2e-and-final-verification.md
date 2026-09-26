---
id: session-hostname-resolution-04
title: E2E and final verification
status: pending
wave: 4
depends_on: [session-hostname-resolution-03]
plan: docs/plans/session-hostname-resolution/plan.md
spec: docs/specs/session-hostname-resolution/spec.md
---

# E2E and final verification

## Inputs

Tasks 01-03 and the [spec](../../specs/session-hostname-resolution/spec.md)
acceptance criteria. Existing E2E patterns:
`apps/web/e2e/tests/auth/auth-screenshots.spec.ts` (auth-mode restart +
account security page), `apps/web/e2e/helpers/auth.ts` (`setupAdmin`, `login`,
`mintPAT`), `apps/web/e2e/helpers/causal-waits.ts` (causal wait primitives),
and `apps/web/e2e/README.md` for projects/commands.

## Project placement (verified against `apps/web/e2e/playwright.config.ts`)

- `tests/auth/*.spec.ts` runs in the **`auth` project**: the backend is
  restarted with auth enabled and the whole API is locked behind a login;
  `mobile-*.spec.ts` files under `tests/auth/` run in the **`mobile-chrome`**
  project instead (the auth project's `testIgnore` excludes them). The new
  spec **must not** claim to run on the default project.

## Implementation

### Desktop spec — `apps/web/e2e/tests/auth/account-security-hostnames.spec.ts`

Setup (mirror `auth-screenshots.spec.ts` for the restart shape, plus
`mobile-users-self-actions.spec.ts` for DB isolation):

- `beforeAll`: `backend.restart({ KANDEV_FEATURES_AUTH: "true",
  KANDEV_DATABASE_PATH: path.join(backend.tmpDir,
  "kandev-hostnames-desktop.db") })` — **an isolated DB path is required**,
  exactly as in the mobile spec. The auth project's worker is serial/shared
  and `backend.restart` preserves the existing SQLite DB, so without an
  isolated path this spec's second login (focus-refetch step), hostname
  cache rows, and admin state leak into later auth-project tests, and
  `setupAdmin` may find the instance already past setup mode. The isolated
  DB also makes this spec's own row-count assertions deterministic across
  runs.
- **Lifecycle (pinned):** call **`setupAdmin(...)` only** to create the
  admin and authenticate the page context — `/auth/setup` already creates
  the admin's session and sets the `kandev_session` cookie on the context,
  so the baseline sessions table has exactly **one** row. Do NOT call
  `login(...)` for the main flow (it would create a second baseline row and
  skew the isolation proof). `login(...)` is used only in the focus-refetch
  step to create the second session row. The page context must carry the
  authenticated session cookie for both the settings page and the
  `afterEach` cleanup PATCH.
- `afterAll`: `backend.restart()` to return to the baseline env (this
  reverts the DB path too).
- `afterEach`: PATCH `resolve_session_hostnames` back to `false` using the
  authenticated API client (the E2E worker does not reset user settings
  between tests).
- **Isolation model (single test, no false claims):** the spec is **one
  `test(...)`** containing the whole flow below — a second test cannot
  assert a fresh one-row baseline because the isolated DB persists across
  tests within the worker (setupAdmin runs once in `beforeAll`, and the
  focus-refetch `login` adds a session that no `afterEach` removes).
  Cross-spec isolation comes from the isolated `KANDEV_DATABASE_PATH`
  itself (other auth-project specs use their own/`backend`'s DB), not from
  an in-spec two-test proof. Within the single test, the row-count
  assertions are: exactly **one** row after `setupAdmin` (fresh DB), and
  exactly **two** after the focus-refetch `login` — these are
  deterministic because the DB is dedicated to this spec.

Flow (assertions are env-robust; DNS answers vary — never assert an exact
hostname). The E2E **never depends on `auth.session.hostname.resolved`
events**: the worker-scoped backend keeps its DB (and therefore the hostname
cache) across tests, and every e2e login comes from the same loopback IP, so
a first-resolution or changed-result event is not guaranteed for any toggle.
Streaming behavior is covered by the resolver/WS-handler/component unit
tests; the E2E validates the UI contract (column, hostname-or-`N/A` cells,
cached rendering on re-enable, persistence, focus-refetch):

0. Before navigation: `const watch = watchWs(page);` then `page.goto(...)`
   (per `causal-waits.ts`, `watchWs` must precede the first `goto`).
1. Navigate to `/settings/account/security`; assert the sessions table has no
   Hostname column (toggle is off by default).
2. Toggle the switch on; assert the column appears and every row's cell is
   either `N/A` or a non-empty hostname immediately (empty always renders
   `N/A`; there is no pending state).
3. Toggle off; assert the column is hidden.
4. Toggle on again; assert the column reappears with values (cached from the
   in-memory map and/or the enable-transition refetch's `hostname` values —
   `N/A` or a non-empty hostname). Do NOT wait for a WS event here: an
   unchanged or fresh-cache result intentionally emits none.
5. Reload the page; assert the toggle is still on and the column is shown
   with hostname-or-`N/A` cells (persistence through user settings).
6. New-session discovery (required, not skippable): with the page still
   open, call `login(...)` **again through the same BrowserContext's request
   client** — that is what creates a second `auth_sessions` row (navigating
   alone only reuses the cookie and adds nothing; the baseline is the single
   `setupAdmin` session, so the table must grow from one row to two). Then:
   a. `const page2 = await ctx.newPage();`
   b. **Explicitly activate page 2 first:** `await page2.bringToFront();`
      — `goto` alone is navigation, not activation, and the focus handler
      on page 1 must deterministically fire from a real blur/focus
      transition.
   c. `await page2.goto("/")` (shared cookies authenticate it); wait for
      the app shell to render on page 2.
   d. Arm a causal wait for the sessions refetch (`watch.waitForHttp` or
      `page.waitForResponse` on `GET /api/v1/auth/sessions`), then
      `await page.bringToFront()` on the original page; await the sessions
      response — this proves the focus handler fired and refetched.
   e. Assert the sessions table now has **two** rows (the `setupAdmin`
      session plus the new login) and the new row's hostname cell is
      hostname-or-`N/A`. If the auth fixture cannot create a second session
      for the same admin, fix the fixture rather than dropping the
      assertion.

### Mobile spec — `apps/web/e2e/tests/auth/mobile-account-security-hostnames.spec.ts`

Runs in the **`mobile-chrome`** project (routed away from the desktop `auth`
project via its `testIgnore`), which does NOT inherit the auth project's
backend restart — the spec must set it up itself, mirroring
`mobile-users-self-actions.spec.ts`:

- `beforeAll`: `backend.restart({ KANDEV_FEATURES_AUTH: "true",
  KANDEV_DATABASE_PATH: path.join(backend.tmpDir,
  "kandev-hostnames-mobile.db") })` (an isolated DB so the hostname cache
  does not leak between the desktop and mobile workers); `afterAll`:
  `backend.restart()` to the baseline env.
- Create a manual context: `browser.newContext({ ...devices["Pixel 5"],
  baseURL: backend.frontendUrl })` (manual contexts do not inherit project
  device options), then `setupAdmin` + `login` before navigating.
- `afterEach`: PATCH `resolve_session_hostnames` back to `false` with the
  authenticated client.

Flow at the Pixel 5 viewport: enable → column appears → cells are
hostname-or-`N/A` (no WS waits, per the desktop flow), disable → hidden,
re-enable → restored. Assert the toggle switch row is reachable/tappable and
the page has no document-level horizontal overflow with the Hostname column
rendered.

## Final verification

```sh
(cd apps/backend && go test -race ./internal/auth/... ./internal/user/... ./internal/gateway/websocket/... ./internal/backendapp/... ./pkg/websocket/...)
make -C apps/backend lint
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm exec vitest run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts lib/ws/handlers/session-hostnames.test.ts lib/state/slices/auth/auth-slice.test.ts components/settings/account/security-settings.test.tsx)
(cd apps/web && pnpm e2e:run --project auth tests/auth/account-security-hostnames.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/auth/mobile-account-security-hostnames.spec.ts)
git diff --check
```

Update the plan's wave table and this task's status to `done` when green.

## Acceptance

1. The desktop spec (auth project) proves default-off, enable → column +
   hostname-or-`N/A` cells (no WS-event waits), disable → hidden, re-enable →
   restored with cached values, reload persistence, and focus-refetch
   discovery of a second login, with authenticated setup/cleanup.
2. The mobile spec (mobile-chrome project) proves the same flow at Pixel 5
   width with no horizontal overflow, using its own auth-enabled backend
   restart (isolated DB), `setupAdmin`/`login`, and authenticated cleanup.
3. All focused backend and frontend checks pass; `git diff --check` is clean.
4. The spec/plan/task statuses are updated to match reality.

## Verification

The commands in "Final verification" are the acceptance gate.

## Dependencies

Tasks 01-03.

## Risks

- Running the spec without the auth project's env/helpers: `GET
  /api/v1/auth/sessions` is behind `RequireRealIdentity`, so an unauthenticated
  page cannot load sessions. Always restart with `KANDEV_FEATURES_AUTH:
  "true"` and log in first.
- Exact-hostname assertions flake across environments (no PTR, `localhost`,
  synthesized `.in-addr.arpa`). Assert column presence/absence and
  hostname-or-`N/A` cell text only; because `N/A` renders immediately there
  is no pending state to wait for.
- Never await `auth.session.hostname.resolved` in E2E: the worker backend
  keeps its cache across tests and e2e logins share one loopback IP, so a
  first-resolution or changed-result event is not guaranteed (fresh cache
  and unchanged results intentionally emit nothing). Streaming is proven by
  unit/component tests.
- Focus-refetch coverage must **call `login` again** through the same
  context's request client (only setup/login create `auth_sessions` rows;
  navigation reuses the cookie), then navigate a second same-context page so
  page 1 really loses focus, then `bringToFront` page 1 and assert the row
  count grew by exactly one. It must not be skippable; fixture limitations
  are fixture bugs.
- The desktop spec must restart with an isolated `KANDEV_DATABASE_PATH`
  (the auth worker is serial/shared and `backend.restart` preserves the DB);
  without it, the focus-refetch second login, hostname cache, and admin
  state leak into later auth-project tests and `setupAdmin` may not be in
  setup mode. Keep the spec a single test with deterministic 1-row → 2-row
  assertions; cross-spec isolation comes from the dedicated DB path, not an
  in-spec two-test proof (the DB persists across tests, so a second test
  cannot claim a fresh baseline).
- The mobile spec runs in the `mobile-chrome` project, which does not
  inherit the auth project's backend restart; it must restart auth-enabled
  with an isolated DB itself (per `mobile-users-self-actions.spec.ts`) or
  the page hits setup/unauthenticated mode and sessions never load.
- A leftover enabled setting leaks into later specs; always reset in
  `afterEach` with the authenticated client.
- The managed E2E runner must rebuild production assets on the first run;
  re-run with `--no-build` only after a successful prior build from the same
  tree.
