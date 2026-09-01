---
id: "02-e2e-guard-mirror"
title: "E2E coverage for guard-matched buttons"
status: done
wave: 2
depends_on: ["01-frontend-disable-last-admin"]
plan: "plan.md"
spec: "../../specs/auth/requirements/self-actions-guard.md"
---

# Task 02: E2E coverage for guard-matched buttons

## Intent

Prove from the browser that the toggle buttons are disabled exactly when the
existing backend last-admin guard would reject the action, and that the
backend behavior itself is unchanged.

## Inputs

- The spec `Scenarios` section.
- E2E auth patterns: `apps/web/e2e/tests/auth/auth-screenshots.spec.ts`
  (serial describe, `backend.restart({ KANDEV_FEATURES_AUTH: "true" })` in
  `beforeAll`, baseline restart in `afterAll`) and
  `apps/web/e2e/helpers/auth.ts` (`setupAdmin`, `login`).
- Row locators: `users-table-row` rows carry `data-user-id`; the toggle
  buttons carry `users-table-toggle-role` / `users-table-toggle-status`.

## Change

New file `apps/web/e2e/tests/auth/users-self-actions.spec.ts`:

1. Serial describe; `beforeAll` restarts the backend with
   `KANDEV_FEATURES_AUTH: "true"`; `afterAll` restarts to baseline.
2. Setup the admin (`setupAdmin`), login the admin context, and create a
   member via `POST /api/v1/users` on `context.request` (shares the session
   cookie). Capture the admin and member ids.
3. Sole-admin UI: open `/settings/system/users`; own row
   `[data-user-id="{adminId}"]` has both toggles `toBeDisabled()`; the member
   row has both `toBeEnabled()`.
4. Existing-guard proof: `PATCH /api/v1/users/{adminId}` with
   `{ "role": "member" }` as the sole admin returns 409 (backend unchanged).
5. Create a second active admin (`POST /api/v1/users`, `role: "admin"`);
   reload the page; the own row's toggles are now `toBeEnabled()`.
6. No-new-behavior proof: `PATCH /api/v1/users/{adminId}` with
   `{ "role": "member" }` now returns 200 (self-demotion still allowed when
   another active admin exists).

## Acceptance

- The spec passes with the production build (the managed runner builds it).
- Test discovery: `pnpm e2e:run --project auth
  tests/auth/users-self-actions.spec.ts` runs exactly the new tests.

## TDD sequence

1. Write the spec (RED: buttons are enabled for the sole admin today).
2. Run it via the auth project; confirm the failures are the expected
   assertions.
3. With Task 01 landed, re-run; confirm green.

## Verification

```bash
cd "$(git rev-parse --show-toplevel)/apps/web" && pnpm e2e:run --project auth tests/auth/users-self-actions.spec.ts
```

## Dependencies

Task 01 (disabled-button state).

## Parallelism

`sequential`. The spec asserts the Task 01 change and the unchanged backend.

## Output contract

Report the spec path, the red and green run results (with the exact command),
blockers, and risks. Update this task and `plan.md` in the same conversation.

## Results

- Spec: `apps/web/e2e/tests/auth/users-self-actions.spec.ts`, one serial test
  in the `auth` project (`backend.restart({ KANDEV_FEATURES_AUTH: "true" })`).
- Red (before Task 01): failed at the sole-admin `toBeDisabled()` assertion on
  `users-table-toggle-role` — button was enabled.
- Green (after Task 01): `cd apps/web && pnpm e2e:run --project auth
  tests/auth/users-self-actions.spec.ts` → 1 passed (7.6s).
- The spec also proves the backend is unchanged: sole-admin self-PATCH
  `{role:"member"}` → 409; after a second active admin, same PATCH → 200.
- Review fixup (luna round 1): `beforeAll` now restarts with a per-file
  `KANDEV_DATABASE_PATH` inside the fixture tmpDir. The worker-scoped
  fixture and `restart()` preserve the SQLite DB and auth setup is
  single-shot, so two auth specs sharing the worker collided regardless of
  admin email; the isolated DB keeps the full `--project auth` suite
  deterministic in either file order. Coverage widened: a disabled-admin row
  is created and asserted enabled while the sole active admin stays disabled
  (catches dropping `status === "active"` from the count or the per-row
  condition), and both active-admin rows are asserted enabled after the
  second admin is added.
- Full auth project run: `pnpm e2e:run --no-build --project auth tests/auth/`
  → exit 0, 15 passed (both spec files in one worker).
- Review fixup (luna round 2, mobile parity): added
  `apps/web/e2e/tests/auth/mobile-users-self-actions.spec.ts` (runs in the
  `mobile-chrome` Pixel 5 project via a `testIgnore` on the desktop `auth`
  project) asserting the same disabled/enabled states at the mobile viewport,
  plus the shared `users-auth-helpers.ts` used by both specs. Mobile run:
  `pnpm e2e:run --no-build --project mobile-chrome
  tests/auth/mobile-users-self-actions.spec.ts` → 1 passed (8.1s).
