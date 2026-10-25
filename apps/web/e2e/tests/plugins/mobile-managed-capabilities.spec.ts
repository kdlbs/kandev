import { expect, test } from "../../fixtures/test-base";
import {
  openInstallDialog,
  PACKAGE_PATH,
  PLUGIN_ID,
  uninstallPluginFixture,
  uploadPackage,
} from "./plugin-test-helpers";

test.describe("managed plugin capabilities on mobile", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallPluginFixture(apiClient);
  });

  test("grant and revoke workspace access with phone-sized controls", async ({ testPage }) => {
    test.setTimeout(90_000);
    await openInstallDialog(testPage);
    await uploadPackage(testPage, PACKAGE_PATH);
    await expect(testPage.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible({ timeout: 15_000 });
    await testPage.goto(`/settings/plugins/${PLUGIN_ID}`);

    const panel = testPage.getByTestId("plugin-capability-approval");
    await expect(panel).toBeVisible();
    const readMessages = panel.getByTestId("plugin-capability-host.v2.read-messages");
    await readMessages.tap();
    await panel.getByTestId("plugin-approval-reason").fill("Allow read-only coordination");

    const save = panel.getByTestId("plugin-approval-save");
    await expect(save).toBeEnabled();
    const saveBox = await save.boundingBox();
    expect(saveBox?.height).toBeGreaterThanOrEqual(44);
    await save.tap();
    await expect(panel.getByTestId("plugin-approval-status")).toContainText("saved");

    await panel.getByTestId("plugin-approval-reason").fill("Remove read access");
    const revoke = panel.getByTestId("plugin-approval-revoke");
    const revokeBox = await revoke.boundingBox();
    expect(revokeBox?.height).toBeGreaterThanOrEqual(44);
    await revoke.tap();
    await expect(panel.getByTestId("plugin-approval-revoked")).toBeVisible();

    const dimensions = await testPage.evaluate(() => ({
      clientWidth: document.documentElement.clientWidth,
      scrollWidth: document.documentElement.scrollWidth,
    }));
    expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth);
  });
});
