import { expect, test } from "../../fixtures/test-base";
import {
  openInstallDialog,
  PACKAGE_PATH,
  PLUGIN_ID,
  uninstallPluginFixture,
  uploadPackage,
} from "../plugins/plugin-test-helpers";

test.describe("Plugin publisher identity", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallPluginFixture(apiClient);
  });

  test("separates upload attribution and keeps it after failed verification", async ({
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

    // The fixture is an upload, so it cannot match the canonical official
    // catalog. Point the built-in source at a non-canonical URL to make the
    // backend's evidence-unavailable response deterministic without mocking
    // the verification endpoint in the browser.
    const restoreMarketplace = await backend.useEnv({
      KANDEV_PLUGIN_MARKETPLACE_URL: "https://example.invalid/plugin-index.json",
    });
    try {
      await testPage.goto("/settings/plugins");
      await row.getByTestId(`plugin-row-link-${PLUGIN_ID}`).click();

      const detail = testPage.getByTestId(`plugin-detail-${PLUGIN_ID}`);
      await expect(detail).toBeVisible();
      const verification = detail.getByTestId("plugin-publisher-verification");
      await expect(verification.getByText("Unverified publisher", { exact: true })).toBeVisible();
      await verification.getByTestId("plugin-verify-publisher").click();
      await expect(verification.getByTestId("plugin-publisher-verification-error")).toBeVisible({
        timeout: 15_000,
      });
      await expect(verification).toContainText("No trusted package is available");
      await expect(verification.getByText("Retry verification", { exact: true })).toBeVisible();

      await testPage.reload();
      const reloaded = testPage.getByTestId("plugin-publisher-verification");
      await expect(reloaded.getByText("Unverified publisher", { exact: true })).toBeVisible();
      await expect(reloaded.getByText("Uploaded file", { exact: true })).toBeVisible();
    } finally {
      await restoreMarketplace();
    }
  });
});
