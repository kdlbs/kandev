/**
 * Docs media generator — NOT a CI assertion. Skipped unless
 * CAPTURE_DOCS_MEDIA=1 is set, so it never runs in the normal e2e shards.
 *
 * When enabled, it installs a real gRPC plugin into the worker's isolated
 * backend (same harness as tests/plugins/plugins.spec.ts) and screenshots
 * the operator-facing surfaces straight into docs/screenshots/plugin-*.png,
 * which the public plugin docs embed as ../screenshots/plugin-*.png (the
 * landing publisher only rewrites/copies images under screenshots/; media/
 * is reserved for <DocsVideo> assets).
 *
 * Regenerate (from apps/web), pointing at the polished kandev-plugin-hello
 * example so the captures match the docs prose (display name "Hello Plugin",
 * "Hello World" nav item, /hello-world route):
 *
 *   CAPTURE_DOCS_MEDIA=1 \
 *   DOCS_PLUGIN_PACKAGE=/abs/path/to/kandev-plugin-hello-1.5.0.tar.gz \
 *   DOCS_PLUGIN_ID=kandev-plugin-hello \
 *   DOCS_PLUGIN_NAV_ID=hello-world \
 *   DOCS_PLUGIN_ROUTE=/hello-world \
 *   pnpm e2e --project=chromium --workers=1 tests/plugins/plugins-docs-screenshots.spec.ts
 *
 * With no DOCS_PLUGIN_* overrides it falls back to the repo's own e2e fixture
 * package (apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz), so the spec is
 * always self-runnable even without the external example checked out.
 */
import fs from "node:fs";
import path from "node:path";
import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";

const CAPTURE = process.env.CAPTURE_DOCS_MEDIA === "1";

const PACKAGE_PATH =
  process.env.DOCS_PLUGIN_PACKAGE ??
  path.resolve(__dirname, "../../../../../apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz");
const PLUGIN_ID = process.env.DOCS_PLUGIN_ID ?? "kandev-plugin-e2e";
const NAV_ITEM_ID = process.env.DOCS_PLUGIN_NAV_ID ?? "e2e-hello";
const PLUGIN_ROUTE = process.env.DOCS_PLUGIN_ROUTE ?? "/plugins/e2e-hello";

const SCREENSHOTS_DIR = path.resolve(__dirname, "../../../../../docs/screenshots");
const VIEWPORT = { width: 1280, height: 860 };

/** Let transient success toasts auto-dismiss so they don't sit in captures. */
async function waitForToastsGone(page: Page): Promise<void> {
  await page
    .locator("[data-sonner-toast]")
    .first()
    .waitFor({ state: "detached", timeout: 8_000 })
    .catch(() => undefined);
}

async function shot(page: Page, name: string): Promise<void> {
  fs.mkdirSync(SCREENSHOTS_DIR, { recursive: true });
  await waitForToastsGone(page);
  await page.screenshot({ path: path.join(SCREENSHOTS_DIR, `plugin-${name}.png`) });
}

test.describe("Plugin docs screenshots", () => {
  test.skip(!CAPTURE, "docs media generator — set CAPTURE_DOCS_MEDIA=1 to run");

  test("captures the operator-facing plugin surfaces", async ({ testPage, apiClient }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize(VIEWPORT);

    // --- Install dialog (upload tab), captured before we actually upload ---
    await testPage.goto("/settings/plugins");
    await testPage.getByTestId("install-plugin-trigger").click();
    await expect(testPage.getByTestId("install-plugin-dialog")).toBeVisible();
    await testPage.getByTestId("install-plugin-tab-upload").click();
    await shot(testPage, "install-dialog");

    // --- Install the package, land on the list with an Active row ---
    await testPage.getByTestId("install-plugin-file-input").setInputFiles(PACKAGE_PATH);
    await testPage.getByTestId("install-plugin-upload-submit").click();
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(pluginRow).toBeVisible({ timeout: 15_000 });
    await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible();
    await expect(testPage.getByTestId("install-plugin-dialog")).toBeHidden();
    await shot(testPage, "settings-list");

    // --- Per-plugin settings page: config_schema form + manifest card ---
    await testPage.getByTestId(`plugin-row-link-${PLUGIN_ID}`).click();
    await expect(testPage).toHaveURL(new RegExp(`/settings/plugins/${PLUGIN_ID}$`));
    await expect(testPage.getByTestId(`plugin-detail-${PLUGIN_ID}`)).toBeVisible();
    await expect(testPage.getByTestId("plugin-manifest-card")).toBeVisible();

    // Capture the pristine first-open form (schema-driven fields at their
    // defaults + the manifest card) — no edits, so no unsaved-changes banner.
    await shot(testPage, "settings-page");

    // --- The plugin's own native route + its sidebar nav item ---
    await testPage.goto("/");
    await testPage.reload();
    const navItem = testPage.getByTestId(`plugin-nav-item-${NAV_ITEM_ID}`);
    await expect(navItem).toBeVisible({ timeout: 15_000 });
    await navItem.click();
    await expect(testPage).toHaveURL(new RegExp(`${PLUGIN_ROUTE}$`));
    await shot(testPage, "native-page");

    // Clean up so a repeat run reinstalls from a clean slate.
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });
});
