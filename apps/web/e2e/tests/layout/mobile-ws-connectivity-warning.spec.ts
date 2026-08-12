import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import {
  captureAppStatusBarSettings,
  restoreAppStatusBarSettings,
  setAppStatusBarEnabled,
  type AppStatusBarSettingsBaseline,
} from "../../helpers/app-status-bar-settings";

test.describe("Mobile WebSocket connectivity warning", () => {
  let baseline: AppStatusBarSettingsBaseline | undefined;

  test.beforeEach(async ({ apiClient }) => {
    baseline = await captureAppStatusBarSettings(apiClient);
    await setAppStatusBarEnabled(apiClient, true);
  });

  test.afterEach(async ({ apiClient }) => {
    await restoreAppStatusBarSettings(apiClient, baseline);
  });

  test("opens a touch-sized connection-only Status drawer while the preference is disabled", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    await setAppStatusBarEnabled(apiClient, false);
    await testPage.goto("/stats");
    await setConnectionIssueSeverity(testPage, "lost");

    await expect(testPage.getByTestId("app-status-bar")).toHaveCount(0);
    const trigger = testPage.getByTestId("app-status-drawer-trigger");
    await expect(trigger).toHaveAttribute(
      "aria-label",
      "Connection lost for at least 10 seconds. Live updates may be stale.",
    );
    await expect(trigger).toHaveAttribute("data-connection-severity", "lost");
    expect((await trigger.boundingBox())?.height).toBeGreaterThanOrEqual(44);

    await trigger.click();
    const drawer = testPage.getByTestId("app-status-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer.locator("[data-status-item-id]")).toHaveCount(1);
    await expect(drawer.getByTestId("app-status-connection")).toHaveAttribute(
      "aria-label",
      "Connection lost for at least 10 seconds. Live updates may be stale.",
    );
    await prCapture.screenshot("mobile-warning-drawer", {
      caption: "Mobile connection-only WebSocket warning drawer",
    });
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );

    await testPage.keyboard.press("Escape");
    await expect(drawer).toBeHidden();
    await expect(trigger).toBeFocused();

    await setConnectionIssueSeverity(testPage, "none");
    await expect(trigger).toHaveCount(0);
  });

  test("keeps the connection-only trigger visible on a coarse-pointer tablet", async ({
    testPage,
    apiClient,
  }) => {
    await testPage.setViewportSize({ width: 900, height: 900 });
    await setAppStatusBarEnabled(apiClient, false);
    await testPage.goto("/stats");
    await setConnectionIssueSeverity(testPage, "unstable");

    const trigger = testPage.getByTestId("app-status-drawer-trigger");
    await expect(trigger).toBeVisible();
    await trigger.tap();
    await expect(testPage.getByTestId("app-status-drawer")).toBeVisible();
  });
});

type E2EStore = {
  getState: () => {
    setConnectionIssueSeverity: (severity: "none" | "unstable" | "lost") => void;
  };
};

async function setConnectionIssueSeverity(page: Page, severity: "none" | "unstable" | "lost") {
  await page.evaluate((nextSeverity) => {
    const store = (window as Window & { __KANDEV_E2E_STORE__?: E2EStore }).__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge missing");
    store.getState().setConnectionIssueSeverity(nextSeverity);
  }, severity);
}
