import { expect, test } from "../../fixtures/test-base";
import {
  captureAppStatusBarSettings,
  restoreAppStatusBarSettings,
  setAppStatusBarEnabled,
  type AppStatusBarSettingsBaseline,
} from "../../helpers/app-status-bar-settings";

test.describe("Mobile App status bar preference", () => {
  let baseline: AppStatusBarSettingsBaseline | undefined;

  test.beforeEach(async ({ apiClient }) => {
    baseline = await captureAppStatusBarSettings(apiClient);
    await setAppStatusBarEnabled(apiClient, true);
  });

  test.afterEach(async ({ apiClient }) => {
    await restoreAppStatusBarSettings(apiClient, baseline);
  });

  test("persists native Status visibility and preserves drawer geometry", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    await testPage.goto("/settings");
    await testPage.getByTestId("settings-index").getByRole("link", { name: "Appearance" }).tap();

    const toggle = testPage.getByRole("switch", { name: "Show status bar" });
    const toggleRow = testPage.getByTestId("app-status-bar-toggle-row");
    const floatingSave = testPage.getByTestId("settings-floating-save");
    await expect(toggle).toHaveAttribute("aria-checked", "true");
    await toggle.scrollIntoViewIfNeeded();
    expect((await toggleRow.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    const toggleBox = await toggle.boundingBox();
    if (!toggleBox) throw new Error("status bar preference target is not visible");
    expect(toggleBox.height).toBeGreaterThanOrEqual(44);
    expect(toggleBox.width).toBeGreaterThanOrEqual(44);
    const tapX = toggleBox.x + toggleBox.width / 2;
    const tapY = toggleBox.y + 5;
    expect(
      await testPage.evaluate(
        ({ x, y }) => document.elementFromPoint(x, y)?.closest("[role='switch']")?.id,
        { x: tapX, y: tapY },
      ),
    ).toBe("show-app-status-bar");
    await testPage.touchscreen.tap(tapX, tapY);
    await expect(toggle).toHaveAttribute("aria-checked", "false");
    await expect(toggle).toHaveAttribute("data-settings-dirty", "true");
    await floatingSave.getByRole("button", { name: "Save changes" }).tap();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.app_status_bar_enabled)
      .toBe(false);

    await testPage.goto("/");
    await testPage.getByRole("button", { name: "Open menu" }).tap();
    await expect(testPage.getByTestId("mobile-home-status-button")).toHaveCount(0);
    await testPage.keyboard.press("Escape");
    await testPage.goto("/stats");
    await expect(testPage.getByTestId("app-status-drawer-trigger")).toHaveCount(0);
    await testPage.reload();
    await expect(testPage.getByTestId("app-status-drawer-trigger")).toHaveCount(0);

    await testPage.goto("/settings/preferences/appearance");
    await expect(toggle).toHaveAttribute("aria-checked", "false");
    await toggle.tap();
    await floatingSave.getByRole("button", { name: "Save changes" }).tap();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.app_status_bar_enabled)
      .toBe(true);

    await testPage.goto("/stats");
    const trigger = testPage.getByTestId("app-status-drawer-trigger");
    await expect(trigger).toBeVisible();
    expect((await trigger.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await trigger.tap();

    const drawer = testPage.getByTestId("app-status-drawer");
    const dialog = testPage.getByRole("dialog", { name: "Status" });
    await expect(drawer).toBeVisible();
    await expect(dialog).toHaveClass(/safe-area-inset-bottom/);
    expect(await drawer.locator("[class*='overflow-y-auto']").count()).toBe(1);
    const viewportHeight = await testPage.evaluate(() => window.innerHeight);
    await expect
      .poll(() => dialog.evaluate((element) => element.getBoundingClientRect().bottom))
      .toBeLessThanOrEqual(viewportHeight);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
    await prCapture.screenshot("status-bar-appearance-mobile", {
      caption: "Mobile Status drawer after enabling the Appearance preference",
    });

    await testPage.keyboard.press("Escape");
    await expect(drawer).toBeHidden();
    await expect(trigger).toBeFocused();
  });
});
