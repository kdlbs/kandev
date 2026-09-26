---
id: "03-e2e-coverage"
title: "E2E coverage"
status: done
wave: 2
depends_on: ["01-contract-and-mapping"]
plan: "plan.md"
spec: "../../specs/plugins/requirements/plugin-nav-sidebar-footer.md"
---

# Task 03: E2E coverage

Prove the `insights` placement end to end through the real plugin install/boot
path: a real plugin registers an `insights` nav item, the real backend boots
it, and the real frontend renders it as a footer icon on desktop and a
Utilities row on a phone. Reuses the existing shared e2e fixture plugin
(`kandev-plugin-e2e`) and its existing `/plugins/e2e-hello` route/page — this
task adds a second nav-item registration pointing at that same page, not a
new page.

## Files to change

1. `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js` — directly
   after the existing:
   ```js
   registry.registerNavItem({
     id: "e2e-hello",
     label: "Hello E2E",
     path: "/plugins/e2e-hello",
     section: "main",
   });
   registry.registerRoute("/plugins/e2e-hello", PluginPage);
   ```
   add a second registration:
   ```js
   registry.registerNavItem({
     id: "e2e-insights-tools",
     label: "E2E Insights Tools",
     path: "/plugins/e2e-hello",
     section: "insights",
   });
   ```
   No new route: both nav items point at the existing `PluginPage`.

2. Repackage the fixture so the tar.gz every e2e spec installs from includes
   the new bundle:
   ```
   cd apps/backend && make e2e-plugin-package
   ```
   This overwrites `apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz`. Do
   not hand-edit that file; only edit `bundle.js` and rebuild through the
   Makefile target (see the target's comment in `apps/backend/Makefile` for
   what it stages/builds/packs).

3. `apps/web/e2e/tests/plugins/plugins.spec.ts` — add one new `test(...)`
   inside the existing `test.describe("Plugins — gRPC plugin
   install/load/live-update/uninstall", ...)` block (its `afterEach` already
   uninstalls via API, so a fresh test here does not need its own cleanup).
   Reuse the file's `openInstallDialog`/`uploadPackage` helpers:
   ```ts
   test("registers an insights-section item as a footer icon, not a rail row", async ({ testPage }) => {
     await openInstallDialog(testPage);
     await uploadPackage(testPage, PACKAGE_PATH);
     await expect(testPage.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible({ timeout: 15_000 });
     await testPage.goto("/");
     await testPage.reload();

     const footerButton = testPage.getByTestId(
       `sidebar-plugin:${PLUGIN_ID}:e2e-insights-tools-button`,
     );
     await expect(footerButton).toBeVisible({ timeout: 15_000 });
     await expect(footerButton).toHaveAccessibleName("E2E Insights Tools");
     await footerButton.click();
     await expect(testPage).toHaveURL(/\/plugins\/e2e-hello$/);

     await expect(testPage.getByTestId("plugin-nav-item-e2e-insights-tools")).toHaveCount(0);
   });
   ```
   Adjust the exact locator/assertion API to match what the rest of the file
   already uses (e.g. confirm `toHaveAccessibleName` is used elsewhere or use
   the row's `getByRole("button", { name: "E2E Insights Tools" })` form
   instead, matching this repo's existing footer-button assertions in
   `app-sidebar-footer.test.tsx`).

4. `apps/web/e2e/tests/plugins/mobile-plugin-nav.spec.ts` — add one new
   `test(...)` inside the existing `test.describe("Mobile plugin
   navigation", ...)` block, following the file's existing inline install
   pattern (goto `/settings/plugins`, install via upload tab, wait for the
   plugin row, `goto("/")`, `reload()`, open the menu sheet):
   ```ts
   test("shows the insights item in the Utilities group, not the Plugins group", async ({ testPage }) => {
     await testPage.goto("/settings/plugins");
     await testPage.getByTestId("install-plugin-trigger").click();
     await testPage.getByTestId("install-plugin-tab-upload").click();
     await testPage.getByTestId("install-plugin-file-input").setInputFiles(PACKAGE_PATH);
     await testPage.getByTestId("install-plugin-upload-submit").click();
     await expect(testPage.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible({ timeout: 30_000 });

     await testPage.goto("/");
     await testPage.reload();
     await testPage.getByRole("button", { name: "Open menu" }).click();

     const utilitiesRow = testPage.getByRole("link", { name: "E2E Insights Tools" });
     await expect(utilitiesRow).toBeVisible();

     const pluginsGroup = testPage.getByTestId("mobile-plugin-nav-section");
     await expect(pluginsGroup.getByText("E2E Insights Tools")).toHaveCount(0);
   });
   ```
   The file's existing `afterEach` already deletes the plugin via API, so no
   extra cleanup is needed. Adjust the exact "open menu" locator to match
   whatever the file's existing tests already use for that button.

## Acceptance

- `apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz` is rebuilt and contains
  the second nav-item registration (verify via
  `tar -xOzf apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz ui/bundle.js | grep e2e-insights-tools`
  or equivalent).
- The new desktop test in `plugins.spec.ts` and the new mobile test in
  `mobile-plugin-nav.spec.ts` pass.
- No existing test in either file (or in `plugins-docs-screenshots.spec.ts`,
  which references the same fixture by `NAV_ITEM_ID`/`PLUGIN_ROUTE`) breaks —
  the new registration is additive and does not change `e2e-hello`'s id,
  label, or route.

## Verification

```
cd apps/backend && make e2e-plugin-package
cd apps/web && pnpm install --frozen-lockfile && pnpm e2e:raw e2e/tests/plugins/plugins.spec.ts e2e/tests/plugins/mobile-plugin-nav.spec.ts e2e/tests/plugins/plugins-docs-screenshots.spec.ts
```

## Files likely touched

- `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`
- `apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz` (generated artifact,
  rebuilt not hand-edited)
- `apps/web/e2e/tests/plugins/plugins.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-plugin-nav.spec.ts`

## Dependencies

01-contract-and-mapping (the fixture's `section: "insights"` registration only
renders in the footer/Utilities group once `pluginDestinations()` maps it
there).

## Parallelism

parallel-safe with 02-frontend-regression-coverage — disjoint files (this task
touches the Go fixture bundle, its packaged artifact, and Playwright spec
files under `apps/web/e2e/**`; task 02 touches only unit test files under
`apps/web/components/**`).

## Inputs

- Spec **Scenarios**: the desktop footer/rail-exclusion scenario and the
  phone Utilities/Plugins-group scenario.
- Spec **Rendered identity**: the footer testid format
  `sidebar-plugin:<pluginId>:<itemId>-button`, and that phone Utilities rows
  carry no `data-testid` (select by visible label there, as the new mobile
  test does).
- `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`'s existing
  `e2e-hello` registration as the direct template.
- `apps/web/e2e/tests/plugins/plugins.spec.ts`'s `openInstallDialog`/
  `uploadPackage` helpers and shared `afterEach`.
- `apps/web/e2e/tests/plugins/mobile-plugin-nav.spec.ts`'s existing inline
  install pattern (no shared helper in that file today).

## Output contract

Summary of the fixture change, the repackage step's output, both new tests,
files touched, exact commands run with outcomes (including the `tar`
verification), and any blockers (e.g. a local platform-specific build issue
with `make e2e-plugin-package`). Update this file's `status` to `done` and
this plan's Wave 2 checkbox and **Verification Results** section in the same
change.

## Results

Added a second `registry.registerNavItem({ id: "e2e-insights-tools", label:
"E2E Insights Tools", path: "/plugins/e2e-hello", section: "insights" })` call
to `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`, right after
the existing `e2e-hello` registration, reusing the same `PluginPage`/route.

Rebuilt the fixture package and verified the new registration made it into
the artifact:
```
cd apps/backend && make e2e-plugin-package
tar -xOzf apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz ui/bundle.js | grep -A5 e2e-insights-tools
```
confirmed the `section: "insights"` registration is present in the packaged
bundle.

Added one test to `apps/web/e2e/tests/plugins/plugins.spec.ts` ("registers an
insights-section item as a footer icon, not a rail row") and one to
`apps/web/e2e/tests/plugins/mobile-plugin-nav.spec.ts` ("shows the insights
item in the Utilities group, not the Plugins group"), both inside their
files' existing `test.describe` blocks per the plan.

Built the binaries/artifacts `e2e/global-setup.ts` requires (none existed
yet in this worktree):
```
make build-backend      # apps/backend/bin/{kandev,mock-agent,...}
make build-web-e2e      # apps/web/dist/ (pseudo-locale build)
```

Ran, from `apps/web`:
- `pnpm e2e:raw e2e/tests/plugins/plugins.spec.ts --project=chromium` — **12
  passed** (all pre-existing tests plus the new one), 51.0s.
- `pnpm e2e:raw e2e/tests/plugins/mobile-plugin-nav.spec.ts --project=mobile-chrome`
  — **5 passed** (all pre-existing tests plus the new one), 24.0s.
- `pnpm e2e:raw e2e/tests/plugins/plugins-docs-screenshots.spec.ts --project=chromium`
  — 2 skipped (gated behind a capture flag not set in this run; pre-existing,
  unrelated to this change — confirms nothing crashed loading that spec with
  the new fixture bundle).
- `pnpm run lint -- e2e/tests/plugins/plugins.spec.ts e2e/tests/plugins/mobile-plugin-nav.spec.ts`
  — 0 errors, 0 warnings.
- `pnpm run typecheck` (apps/web) — clean.

No teardown needed beyond what the suites already do themselves (each test's
plugin install is cleaned up by the files' existing `afterEach`/backend reset).
No security/trust or external side-effect boundaries beyond the existing
plugin-install flow the other tests in these files already exercise.

Files touched matched **Files likely touched** exactly (plus the two build
artifacts `apps/backend/bin/*` and `apps/web/dist/*`, which are gitignored
build output, not tracked changes).
