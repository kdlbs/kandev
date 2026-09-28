import { expect, test } from "../../fixtures/test-base";
import {
  installFixturePlugin,
  PLUGIN_ID,
  uninstallFixturePlugin,
} from "../../helpers/plugin-fixture";
import {
  captureAppStatusBarSettings,
  restoreAppStatusBarSettings,
  setAppStatusBarEnabled,
  type AppStatusBarSettingsBaseline,
} from "../../helpers/app-status-bar-settings";

test.describe("Plugin Action compatibility", () => {
  let statusBarBaseline: AppStatusBarSettingsBaseline | undefined;
  let metricsBaseline: { show_in_topbar: boolean; simplified?: boolean } | undefined;

  test.afterEach(async ({ apiClient }) => {
    try {
      await uninstallFixturePlugin(apiClient);
    } finally {
      try {
        if (metricsBaseline) {
          const restoreResponse = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
            system_metrics_display: metricsBaseline,
          });
          expect(restoreResponse.ok).toBe(true);
          const restored = await apiClient.getUserSettings();
          expect(restored.settings.system_metrics_display).toMatchObject(metricsBaseline);
        }
      } finally {
        await restoreAppStatusBarSettings(apiClient, statusBarBaseline);
        statusBarBaseline = undefined;
        metricsBaseline = undefined;
      }
    }
  });

  test("keeps mixed legacy and standard controls isolated through disable, enable, and saved order", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    statusBarBaseline = await captureAppStatusBarSettings(apiClient);
    const { settings } = await apiClient.getUserSettings();
    metricsBaseline = (settings.system_metrics_display as
      | { show_in_topbar: boolean; simplified?: boolean }
      | undefined) ?? { show_in_topbar: false };
    await installFixturePlugin(testPage);
    await setAppStatusBarEnabled(apiClient, true);
    const leftId = `plugin:${PLUGIN_ID}:app-status-bar-left:0`;
    const rightId = `plugin:${PLUGIN_ID}:app-status-bar-right:0`;
    const settingsResponse = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      system_metrics_display: { show_in_topbar: true },
      app_status_bar_order: {
        left_item_ids: [rightId, "builtin:metrics"],
        right_item_ids: ["builtin:connection", leftId],
      },
    });
    expect(settingsResponse.ok).toBe(true);

    await testPage.goto("/tasks");
    const legacyHostButton = testPage.locator("#hello-main-top-bar");
    const rawLegacyButton = testPage.getByTestId("e2e-legacy-raw-topbar-action");
    const legacyDisclosure = testPage.getByTestId("e2e-legacy-preview-trigger");
    const disclosureContent = testPage.getByTestId("e2e-legacy-preview-content");
    const standardAction = testPage.getByTestId("e2e-main-topbar-action");
    await expect(legacyHostButton).toBeVisible();
    await expect(legacyHostButton).toHaveAttribute("data-slot", "button");
    await expect(rawLegacyButton).toBeVisible();
    await expect(standardAction).toHaveAttribute("data-slot", "surface-action");
    await expect(standardAction).toHaveAttribute("data-surface", "topbar");

    const [hostButtonBox, rawButtonBox] = await Promise.all([
      legacyHostButton.boundingBox(),
      rawLegacyButton.boundingBox(),
    ]);
    expect(hostButtonBox).not.toBeNull();
    expect(rawButtonBox).not.toBeNull();
    expect(hostButtonBox!.height).toBe(24);
    expect(hostButtonBox!.width).toBe(24);
    expect(rawButtonBox!.height).toBe(28);
    expect(rawButtonBox!.width).toBeGreaterThanOrEqual(48);

    await rawLegacyButton.click();
    await expect(rawLegacyButton).toHaveAttribute("data-activated", "true");
    await legacyDisclosure.click();
    await expect(legacyDisclosure).toHaveAttribute("aria-expanded", "true");
    await expect(disclosureContent).toBeVisible();
    await standardAction.click();
    await expect(standardAction).toHaveAttribute("aria-pressed", "true");
    await expect(rawLegacyButton).toHaveAttribute("data-activated", "true");
    await expect(legacyDisclosure).toHaveAttribute("aria-expanded", "true");

    const readStatusOrder = () =>
      testPage
        .getByTestId("app-status-bar")
        .locator("[data-status-item-id]")
        .evaluateAll((rows) => rows.map((row) => row.getAttribute("data-status-item-id")));
    const expectedStatusOrder = await readStatusOrder();
    expect(expectedStatusOrder.filter((id) => id === leftId || id === rightId)).toEqual([
      rightId,
      leftId,
    ]);
    await testPage.reload();
    expect(await readStatusOrder()).toEqual(expectedStatusOrder);
    await expect(testPage.getByTestId("e2e-main-topbar-action")).toBeVisible();

    await testPage.goto("/settings/plugins");
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await pluginRow.getByRole("button", { name: "Disable" }).click();
    await expect(pluginRow.getByText("Disabled", { exact: true })).toBeVisible();
    await testPage.goto("/tasks");
    await expect(testPage.getByTestId("e2e-main-topbar-action")).toHaveCount(0);
    await expect(testPage.getByTestId("e2e-legacy-raw-topbar-action")).toHaveCount(0);
    await expect(testPage.locator(`[data-status-item-id="${leftId}"]`)).toHaveCount(0);
    await expect(testPage.locator(`[data-status-item-id="${rightId}"]`)).toHaveCount(0);

    await testPage.goto("/settings/plugins");
    await pluginRow.getByRole("button", { name: "Enable" }).click();
    await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible();
    await testPage.goto("/tasks");
    await expect(testPage.getByTestId("e2e-main-topbar-action")).toBeVisible();
    await expect(testPage.getByTestId("e2e-legacy-raw-topbar-action")).toBeVisible();
    expect(await readStatusOrder()).toEqual(expectedStatusOrder);
    await expect(testPage.getByTestId("e2e-main-topbar-action")).toHaveAttribute(
      "aria-pressed",
      "false",
    );
    await expect(testPage.getByTestId("e2e-legacy-preview-trigger")).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });
});
