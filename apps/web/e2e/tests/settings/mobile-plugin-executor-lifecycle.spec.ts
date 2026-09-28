import { expect, test } from "../../fixtures/test-base";
import {
  openInstallDialog,
  PACKAGE_PATH,
  PLUGIN_ID,
  uninstallPluginFixture,
  uploadPackage,
} from "../plugins/plugin-test-helpers";

test.describe("Phone plugin executor lifecycle", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallPluginFixture(apiClient);
  });

  test("shows that remote environments can remain after a provider is disabled", async ({
    testPage,
  }) => {
    await openInstallDialog(testPage);
    await uploadPackage(testPage, PACKAGE_PATH);
    const row = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(row).toBeVisible();

    await testPage.route(`**/api/plugins/${PLUGIN_ID}/disable`, async (route) => {
      const response = await route.fetch();
      const body = (await response.json()) as {
        disabled: boolean;
        remote_resources_may_remain: boolean;
      };
      await route.fulfill({
        status: response.status(),
        json: { ...body, remote_resources_may_remain: true },
      });
    });

    await row.getByRole("button", { name: "Disable", exact: true }).tap();
    await expect(
      testPage.getByText(
        "Remote environments can keep running after you disable this plugin. Re-enable it and clean up its task environments before uninstalling.",
        { exact: true },
      ),
    ).toBeVisible();
    await expect(row.getByRole("button", { name: "Enable", exact: true })).toBeVisible();
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
  });
});
