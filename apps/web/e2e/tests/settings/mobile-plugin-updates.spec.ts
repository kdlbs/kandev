import { expect, test } from "../../fixtures/test-base";
import {
  openInstallDialog,
  PACKAGE_PATH,
  PLUGIN_ID,
  uninstallPluginFixture,
  uploadPackage,
} from "../plugins/plugin-test-helpers";

// @covers AC-UI-CONTROL-SIZING-001.3, AC-UI-CONTROL-SIZING-001.4, AC-UI-CONTROL-SIZING-001.5, AC-UI-CONTROL-SIZING-001.8
test.describe("Mobile plugin updates", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallPluginFixture(apiClient);
  });

  test("checks and updates marketplace state from the mobile settings flow", async ({
    testPage,
    prCapture,
  }) => {
    await openInstallDialog(testPage);
    await uploadPackage(testPage, PACKAGE_PATH);
    await expect(testPage.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible();

    let catalogRequests = 0;
    let refreshRequests = 0;
    await testPage.route("**/api/plugins/marketplace", async (route) => {
      catalogRequests += 1;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          plugins: [
            {
              id: PLUGIN_ID,
              name: "E2E Hello",
              description: "",
              author: "kandev",
              categories: [],
              icon_url: "",
              repo_url: "",
              version: "9.9.9",
              min_kandev_version: "",
              package_url: "https://example.invalid/kandev-plugin-e2e-9.9.9.tar.gz",
              package_sha256: "",
              stars: 0,
              updated_at: new Date(0).toISOString(),
              install_state: "update_available",
              installed_version: "1.0.0",
              source_id: "official",
              source_name: "Kandev Official",
            },
          ],
          sources: [
            {
              id: "official",
              name: "Kandev Official",
              url: "https://example.invalid",
              enabled: true,
              builtin: true,
              healthy: true,
            },
          ],
        }),
      });
    });
    await testPage.route("**/api/plugins/marketplace/refresh", async (route) => {
      refreshRequests += 1;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ refreshed: true }),
      });
    });

    await testPage.goto("/settings");
    await testPage.getByRole("link", { name: "Plugins" }).tap();
    await expect(testPage).toHaveURL(/\/settings\/plugins$/);
    await expect.poll(() => catalogRequests).toBe(1);

    const checkButton = testPage.getByTestId("plugins-check-updates-button");
    await expect(checkButton).toBeVisible();
    const buttonBox = await checkButton.boundingBox();
    expect(buttonBox).not.toBeNull();
    expect(buttonBox!.height).toBeGreaterThanOrEqual(44);

    await prCapture.screenshot("plugin-settings", { caption: "Plugin settings on a phone" });
    const syncBox = await testPage.getByTestId("plugins-sync-button").boundingBox();
    const installBox = await testPage.getByTestId("install-plugin-trigger").boundingBox();
    const headingBox = await testPage
      .getByRole("heading", { name: "Installed plugins", exact: true })
      .boundingBox();
    expect(syncBox).not.toBeNull();
    expect(installBox).not.toBeNull();
    expect(headingBox).not.toBeNull();
    expect(syncBox!.y).toBeCloseTo(buttonBox!.y, 0);
    expect(installBox!.y).toBeLessThan(headingBox!.y + headingBox!.height);
    expect(installBox!.width).toBeLessThan(testPage.viewportSize()!.width / 2);

    await checkButton.tap();

    await expect.poll(() => refreshRequests).toBe(1);
    await expect.poll(() => catalogRequests).toBe(2);
    await expect(testPage.getByTestId("plugins-updates-last-checked")).toBeVisible();

    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    const updateButton = pluginRow.getByTestId(`plugin-update-${PLUGIN_ID}`);
    const settingsLink = pluginRow.getByTestId(`plugin-settings-link-${PLUGIN_ID}`);
    await expect(updateButton).toBeVisible();
    await expect(updateButton).toHaveAttribute("data-variant", "default");
    await expect(settingsLink).toBeVisible();
    await expect(settingsLink).toHaveText("Settings");
    const rowBox = await pluginRow.boundingBox();
    const updateBox = await updateButton.boundingBox();
    expect(rowBox).not.toBeNull();
    expect(updateBox).not.toBeNull();
    expect(updateBox!.height).toBeGreaterThanOrEqual(44);
    const settingsBox = await settingsLink.boundingBox();
    expect(settingsBox).not.toBeNull();
    expect(settingsBox!.height).toBeGreaterThanOrEqual(44);
    expect(updateBox!.x + updateBox!.width).toBeLessThanOrEqual(rowBox!.x + rowBox!.width + 1);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    for (const width of [320, 767, 768]) {
      await testPage.setViewportSize({ width, height: 851 });
      const sync = testPage.getByTestId("plugins-sync-button");
      const install = testPage.getByTestId("install-plugin-trigger");
      await expect(sync).toBeVisible();
      await expect(install).toBeVisible();
      const [syncBounds, checkBounds, installBounds] = await Promise.all([
        sync.boundingBox(),
        checkButton.boundingBox(),
        install.boundingBox(),
      ]);
      expect(syncBounds!.y).toBeCloseTo(checkBounds!.y, 0);
      for (const box of [syncBounds!, checkBounds!, installBounds!]) {
        expect(box.height).toBeGreaterThanOrEqual(44);
        expect(box.x).toBeGreaterThanOrEqual(0);
        expect(box.x + box.width).toBeLessThanOrEqual(width);
      }
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth),
      ).toBeLessThanOrEqual(width);
    }
  });
});
