import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";

test.describe("Mobile WebSocket connectivity warning", () => {
  test("opens a touch-sized connection-only Status drawer while the feature is disabled", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.goto("/stats");
    await setAppStatusBarEnabled(testPage, false);
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
  }) => {
    await testPage.setViewportSize({ width: 900, height: 900 });
    await testPage.goto("/stats");
    await setAppStatusBarEnabled(testPage, false);
    await setConnectionIssueSeverity(testPage, "unstable");

    const trigger = testPage.getByTestId("app-status-drawer-trigger");
    await expect(trigger).toBeVisible();
    await trigger.tap();
    await expect(testPage.getByTestId("app-status-drawer")).toBeVisible();
  });
});

type E2EStore = {
  getState: () => {
    features: Record<string, boolean>;
    setFeatures: (features: Record<string, boolean>) => void;
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

async function setAppStatusBarEnabled(page: Page, enabled: boolean) {
  await page.evaluate((nextEnabled) => {
    const store = (window as Window & { __KANDEV_E2E_STORE__?: E2EStore }).__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge missing");
    const state = store.getState();
    state.setFeatures({ ...state.features, appStatusBar: nextEnabled });
  }, enabled);
}
