---
id: "04-e2e-relative-last-seen"
title: "E2E coverage for relative last seen"
status: done
wave: 4
depends_on: ["03-security-page-relative-last-seen"]
plan: "plan.md"
spec: "../../specs/ui/requirements/relative-last-seen.md"
---

# Task 04: E2E coverage for relative last seen (desktop + mobile + cross-tab)

## Acceptance

- **Desktop** (`auth` project, Desktop Chrome): the file groups its tests in
  `test.describe.serial` (shared worker backend/DB and backend restart still require serial
  ordering), restarts the worker backend with auth on and its OWN `KANDEV_DATABASE_PATH`, creates
  the administrator in `beforeAll` so a focused test run is independent, then logs in per test.
  Opening `/settings/account/security` shows the Last seen select. Switching to Relative time
  updates the local draft. The shared Save action persists the setting. A relative label appears in
  the Last seen column, and hovering it reveals a tooltip containing the absolute timestamp.
- **Cross-tab** (same desktop spec, serial): two browser contexts on the same backend, BOTH
  authenticated (Playwright contexts have isolated cookie stores and the auth helpers set cookies
  per context, so context B must `login` separately — or be created from context A's captured
  `storageState` — or it cannot PATCH the setting or receive the user-scoped WS event). Arm
  `watchWs(pageA)` BEFORE `pageA.goto` (it only sees sockets opened after it is called), and arm
  the subscription wait itself before the same `goto` (`const subscriptionAck =
  watcher.waitForResponse("user.subscribe")`), then await `subscriptionAck` after navigation before
  tab B mutates: frames are not buffered and `waitForResponse` only records request ids once armed,
  so a wait created after `goto` can miss the ACK entirely. Tab B switches the setting, tab A
  observes the update via WS without a reload — causal, not timing-based.
- **Mobile** (`mobile-chrome` project, Pixel 5): the spec spreads `...devices["Pixel 5"]` into a
  manual `browser.newContext` (manual contexts do not inherit project device options) and asserts
  the resulting viewport width; with its own DB + `setupAdmin`, the Last seen select is operated
  with touch-native `tap()` on the trigger and the Relative time option, then saved with the shared
  Save action. Relative labels render without hover, there is no horizontal document overflow, and
  tapping a relative label opens a drawer with the exact timestamp. The Last seen trigger and
  options get a mobile-appropriate touch target (min-h-11 / 44px at phone widths via responsive
  classes on this instance — the shared Select defaults to a 28px trigger and items — per
  /mobile-parity), and the mobile spec asserts the trigger's and option's bounding boxes are >=
  44px in the active dimension.
- The original setting is restored in `finally` and the baseline backend is restarted in
  `afterAll` in both specs so the worker is reusable.

## Verification

```bash
cd apps/web && pnpm e2e:run --project auth tests/auth/relative-last-seen.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/auth/mobile-relative-last-seen.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/auth/relative-last-seen.spec.ts` (new; `backendFixture`, `setupAdmin`/`login`
  from `e2e/helpers/auth.ts`, own DB path under `backend.tmpDir`, `finally` setting restore via raw
  PATCH, `afterAll` baseline restart; two-context cross-tab scenario)
- `apps/web/e2e/tests/auth/mobile-relative-last-seen.spec.ts` (new; MUST be named `mobile-*` so the
  `mobile-chrome` project routes it away from the desktop `auth` project; own DB + `setupAdmin`;
  `...devices["Pixel 5"]` spread into `browser.newContext` + viewport width assertion; `afterAll`
  baseline restart, mirroring `mobile-users-self-actions.spec.ts`)

## Dependencies

Task 03 (UI exists to exercise).

## Inputs

- Spec "Scenarios" (switch, tooltip, persistence, restore, cross-tab) and "Failure modes" (touch
  path)
- Existing precedent: `users-self-actions.spec.ts` (own-DB isolation rationale),
  `auth-lifecycle.spec.ts` (serial auth project), `auth-screenshots.spec.ts` (security page
  visit), `mobile-users-self-actions.spec.ts` (mobile auth project + own DB + Pixel 5 device
  spread), `settings-manual-save.spec.ts` (restore settings via raw PATCH), `/mobile-parity`
  naming and routing rules

## Output contract

Return a compact handoff capsule with acceptance status, exact E2E command/results, risk tags,
failure artifacts when applicable, uncertainties, and set this task to `done`.
