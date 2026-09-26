import { expect, test } from "../../fixtures/test-base";
import { expectTouchControl } from "../../helpers/control-sizing";
import {
  openInstallDialog,
  PACKAGE_PATH,
  PLUGIN_ID,
  uninstallPluginFixture,
  uploadPackage,
} from "../plugins/plugin-test-helpers";

test.describe("Plugin publisher identity on mobile", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallPluginFixture(apiClient);
  });

  test("keeps unverified attribution readable and verification reachable by tap", async ({
    testPage,
    backend,
  }) => {
    test.setTimeout(120_000);

    await openInstallDialog(testPage);
    await uploadPackage(testPage, PACKAGE_PATH);

    const row = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(row).toBeVisible({ timeout: 30_000 });
    await expect(row.getByText("Unverified publisher", { exact: true })).toBeVisible();
    await expect(row.getByText("Uploaded file", { exact: true })).toBeVisible();
    await expect(row.getByText("kandev", { exact: true })).toBeVisible();
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    const restoreMarketplace = await backend.useEnv({
      KANDEV_PLUGIN_MARKETPLACE_URL: "https://example.invalid/plugin-index.json",
    });
    try {
      await testPage.goto("/settings/plugins");
      await row.getByTestId(`plugin-row-link-${PLUGIN_ID}`).tap();

      const detail = testPage.getByTestId(`plugin-detail-${PLUGIN_ID}`);
      await expect(detail).toBeVisible();
      const verification = detail.getByTestId("plugin-publisher-verification");
      const verifyButton = verification.getByTestId("plugin-verify-publisher");
      await expectTouchControl(verifyButton);
      await verifyButton.tap();
      await expect(verification.getByTestId("plugin-publisher-verification-error")).toBeVisible({
        timeout: 15_000,
      });
      await expect(verification).toContainText("No trusted package is available");
      const retry = verification.getByText("Retry verification", { exact: true });
      await expectTouchControl(retry);
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
    } finally {
      await restoreMarketplace();
    }
  });
});
