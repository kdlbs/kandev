import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import {
  INTEGRATION_LINK,
  INTEGRATION_NODE,
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
  const legacy = (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace![
    seedData.workspaceId
  ];
  await apiClient.saveUserSettings({
    sidebar_layout_state: {
      workspace_id: seedData.workspaceId,
      expected_revision: legacy.revision,
      layout: {
        ...legacy,
        navigation_expanded: true,
        nodes: [
          ...legacy.nodes.filter((node) => node.destination_id !== "integrations"),
          {
            id: "integrations",
            kind: "shortcuts",
            visible: true,
            name: "Saved links",
            shortcuts: [
              { id: "saved-integration", target: { kind: "destination", id: INTEGRATION_NODE } },
            ],
          },
        ],
      },
    },
  });
  await page.reload();
  const legacySection = sidebar.getByTestId("sidebar-node-integrations-1");
  const legacyLink = legacySection.getByTestId(INTEGRATION_LINK);
  await expect(legacyLink).toBeVisible();
  await legacySection.click({ button: "right" });
  const hiddenSave = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/user/settings") &&
      response.request().method() === "PATCH" &&
      response
        .request()
        .postDataJSON()
        ?.sidebar_layout_state?.layout?.nodes.some(
          (node: { destination_id?: string; visible: boolean }) =>
            node.destination_id === "integrations" && !node.visible,
        ),
  );
  await page.getByTestId("sidebar-visibility-integrations-1").click();
  expect((await hiddenSave).status()).toBe(200);
  await expect(legacySection).toHaveCount(0);
  await expect(section.getByRole("link", { name: "E2E Integration", exact: true })).toBeVisible();
  await expect
    .poll(
      async () =>
        (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace![
          seedData.workspaceId
        ].nodes.find((node) => node.destination_id === "integrations")?.visible,
    )
    .toBe(false);
  await page.keyboard.press("Escape");
  await page.reload();
  await sidebar
    .getByTestId("sidebar-node-home")
    .getByRole("link", { name: "Home", exact: true })
    .click({ button: "right" });
  await expect(page.getByTestId("sidebar-customize-menu")).toBeVisible();
  const shownSave = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/user/settings") &&
      response.request().method() === "PATCH" &&
      response
        .request()
        .postDataJSON()
        ?.sidebar_layout_state?.layout?.nodes.some(
          (node: { destination_id?: string; visible: boolean }) =>
            node.destination_id === "integrations" && node.visible,
        ),
  );
  await page.getByTestId("sidebar-visibility-integrations-1").click();
  expect((await shownSave).status()).toBe(200);
  await expect(legacyLink).toBeVisible();
  const persisted = (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace![
    seedData.workspaceId
  ];
  expect(new Set(persisted.nodes.map((node) => node.id)).size).toBe(persisted.nodes.length);
  expect(persisted.nodes.find((node) => node.id === "integrations")?.kind).toBe("shortcuts");

  await page.getByRole("button", { name: "Collapse sidebar", exact: true }).click();
  await sidebar.getByRole("button", { name: "Integrations", exact: true }).click();
  await expect(page.getByTestId(INTEGRATION_LINK)).toBeVisible();
});

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-001.10
// Regression for the wider consumer of the same integration projection.
test("tablet listing menus keep integration plugin links expanded", async ({
  tabletTestPage: page,
}) => {
  test.setTimeout(120_000);
  await installFixturePlugin(page);
  await page.goto("/tasks");
  await page.getByRole("button", { name: "Open menu", exact: true }).click();
  const menu = page.getByRole("dialog", { name: "Menu", exact: true });
  await expect(menu).toBeVisible();
  await expect(menu.getByRole("button", { name: "Integrations", exact: true })).toHaveCount(0);
  const link = menu.getByTestId(INTEGRATION_LINK);
  await link.scrollIntoViewIfNeeded();
  await expect(link).toBeVisible();
  await expect(menu.getByTestId(INTEGRATION_LINK)).toHaveCount(1);
  await link.click();
  await expect(page).toHaveURL(/\/plugins\/e2e-hello$/);
  await expect(menu).toBeHidden();
});
