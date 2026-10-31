import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import {
  INTEGRATION_LINK,
  setIntegrationVisibility,
} from "../../helpers/integration-plugin-layout";

test.afterEach(async ({ apiClient }) => {
  await uninstallFixturePlugin(apiClient);
});

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-001.10, AC-UI-SIDEBAR-CUSTOMIZATION-001.11, AC-UI-SIDEBAR-CUSTOMIZATION-005.2, AC-UI-SIDEBAR-CUSTOMIZATION-005.3
test("opens integration plugins from the phone disclosure and restores visibility", async ({
  testPage: page,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await installFixturePlugin(page);
  await page.goto("/");
  await page.getByTestId("app-nav-trigger").click();
  const menu = page.getByTestId("app-nav-sheet");
  const toggle = menu.getByRole("button", { name: "Integrations", exact: true });
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
  await expect(menu.getByTestId(INTEGRATION_LINK)).toHaveCount(0);
  await toggle.tap();
  const link = menu.getByTestId(INTEGRATION_LINK);
  await expect(link).toBeVisible();
  await expect(menu.getByRole("link", { name: "E2E Integration", exact: true })).toHaveCount(1);
  const box = (await link.boundingBox())!;
  const bounds = (await menu.boundingBox())!;
  expect(box.height).toBeGreaterThanOrEqual(44);
  expect(box.x).toBeGreaterThanOrEqual(bounds.x);
  expect(box.x + box.width).toBeLessThanOrEqual(bounds.x + bounds.width);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await prCapture.screenshot("mobile-integration-plugin-grouping", {
    caption: "Phone navigation groups plugin links inside the Integrations disclosure",
  });
  await link.tap();
  await expect(page).toHaveURL(/\/plugins\/e2e-hello$/);
  await expect(menu).toBeHidden();
  await setIntegrationVisibility(apiClient, seedData.workspaceId, false);
  await page.goto("/");
  await page.reload();
  await page.getByTestId("app-nav-trigger").click();
  await toggle.tap();
  await expect(menu.getByRole("link", { name: "E2E Integration", exact: true })).toHaveCount(0);
  await expect(menu.getByTestId("mobile-integration-settings")).toBeVisible();
});
