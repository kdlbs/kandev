import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";

test.describe("WebSocket connectivity warning", () => {
  test("shows one bottom-bar warning while the app status bar is enabled", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.goto("/");
    await setConnectionIssueSeverity(testPage, "unstable");

    const warning = testPage.getByTestId("app-status-connection");
    await expect(warning).toHaveAttribute(
      "aria-label",
      "Connection unstable. Reconnecting to Kandev.",
    );
    await expect(warning).toHaveAttribute("data-connection-severity", "unstable");
    await expect(testPage.getByTestId("sidebar-connection-warning")).toHaveCount(0);
    await prCapture.screenshot("desktop-warning", {
      caption: "Desktop yellow WebSocket connectivity warning",
    });

    await setConnectionIssueSeverity(testPage, "lost");
    await expect(warning).toHaveAttribute(
      "aria-label",
      "Connection lost for at least 10 seconds. Live updates may be stale.",
    );

    await setConnectionIssueSeverity(testPage, "none");
    await expect(warning).toHaveCount(0);
  });

  test("uses the sidebar fallback when the app status bar is disabled", async ({ testPage }) => {
    await testPage.goto("/");
    await setAppStatusBarEnabled(testPage, false);
    await setConnectionIssueSeverity(testPage, "unstable");

    await expect(testPage.getByTestId("app-status-bar")).toHaveCount(0);
    const warning = testPage.getByTestId("sidebar-connection-warning");
    await expect(warning).toHaveAttribute(
      "aria-label",
      "Connection unstable. Reconnecting to Kandev.",
    );
    await expect(warning).toHaveAttribute("data-connection-severity", "unstable");
    await warning.focus();
    await expect(testPage.getByRole("tooltip")).toContainText("Connection unstable");

    await setConnectionIssueSeverity(testPage, "none");
    await expect(warning).toHaveCount(0);
  });

  test("keeps a connection-only warning reachable on tablet widths", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 700, height: 900 });
    await testPage.goto("/stats");
    await setAppStatusBarEnabled(testPage, false);
    await setConnectionIssueSeverity(testPage, "unstable");

    await expect(testPage.getByTestId("app-status-bar")).toHaveCount(0);
    const trigger = testPage.getByTestId("app-status-drawer-trigger");
    await expect(trigger).toBeVisible();
    await expect(trigger).toHaveAttribute(
      "aria-label",
      "Connection unstable. Reconnecting to Kandev.",
    );

    await trigger.click();
    const drawer = testPage.getByTestId("app-status-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer.locator("[data-status-item-id]")).toHaveCount(1);
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
