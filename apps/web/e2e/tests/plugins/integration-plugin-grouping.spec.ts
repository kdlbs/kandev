import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import {
  INTEGRATION_LINK,
  pinIntegration,
  setIntegrationVisibility,
} from "../../helpers/integration-plugin-layout";

test.afterEach(async ({ apiClient }) => {
  await uninstallFixturePlugin(apiClient);
});

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-001.10, AC-UI-SIDEBAR-CUSTOMIZATION-001.11, AC-UI-SIDEBAR-CUSTOMIZATION-001.12
test("groups integration plugin navigation and preserves saved visibility", async ({
  testPage: page,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await installFixturePlugin(page);
  await page.goto("/");
  const sidebar = page.getByTestId("app-sidebar");
  const section = sidebar.getByTestId("sidebar-node-integrations");
  const toggle = section.getByRole("button", { name: "Integrations", exact: true });
  if ((await toggle.getAttribute("aria-expanded")) !== "true") await toggle.click();
  const link = section.getByTestId(INTEGRATION_LINK);
  await expect(link).toBeVisible();
  await expect(sidebar.getByTestId(INTEGRATION_LINK)).toHaveCount(1);
  await prCapture.screenshot("desktop-integration-plugin-grouping", {
    caption: "Integration plugins remain inside the expanded Integrations section",
  });
  await toggle.click();
  await expect(sidebar.getByTestId(INTEGRATION_LINK)).toBeHidden();
  await toggle.click();
  await link.click();
  await expect(page).toHaveURL(/\/plugins\/e2e-hello$/);
  await page.goto("/");

  await setIntegrationVisibility(apiClient, seedData.workspaceId, false);
  await page.reload();
  await expect(sidebar.getByTestId(INTEGRATION_LINK)).toHaveCount(0);
  await setIntegrationVisibility(apiClient, seedData.workspaceId, true);
  await page.reload();
  await expect(link).toBeVisible();
  await pinIntegration(apiClient, seedData.workspaceId);
  await setIntegrationVisibility(apiClient, seedData.workspaceId, false, true);
  await page.reload();
  await expect(sidebar.getByTestId(INTEGRATION_LINK)).toHaveCount(0);
  const pinned = sidebar.getByTestId("sidebar-node-pinned-integrations");
  await expect(pinned.getByRole("link", { name: "E2E Integration", exact: true })).toBeVisible();
  await setIntegrationVisibility(apiClient, seedData.workspaceId, true, true);
  await page.reload();
  await expect(link).toBeVisible();

  const { settings } = await apiClient.getUserSettings();
  const revision = settings.sidebar_layouts_by_workspace![seedData.workspaceId].revision;
  await apiClient.saveUserSettings({
    sidebar_layout_state: {
      workspace_id: seedData.workspaceId,
      expected_revision: revision,
      layout: null,
    },
  });
  await page.reload();
  await expect(link).toBeVisible();
  await expect(pinned).toHaveCount(0);
  await page.getByRole("button", { name: "Collapse sidebar", exact: true }).click();
  await sidebar.getByRole("button", { name: "Integrations", exact: true }).click();
  await expect(page.getByTestId(INTEGRATION_LINK)).toBeVisible();
});
