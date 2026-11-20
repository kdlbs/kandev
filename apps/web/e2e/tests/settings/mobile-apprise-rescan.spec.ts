import { test, expect } from "../../fixtures/test-base";
import { APPRISE_PROVIDER_NAME, routeAppriseRescans } from "./apprise-rescan-helpers";
import { waitForHttp } from "../../helpers/causal-waits";

test.describe("Mobile Apprise rescan", () => {
  test("detects Apprise with a touch-sized action and no horizontal overflow", async ({
    testPage,
    prCapture,
  }) => {
    const { startRescan } = await routeAppriseRescans(testPage, [true]);
    const providersLoaded = waitForHttp(testPage, "GET", /\/api\/v1\/notification-providers$/);
    await testPage.goto("/settings/preferences/notifications");
    await providersLoaded;

    const rescan = testPage.getByTestId("apprise-rescan");
    await expect(rescan).toBeVisible();
    const beforeBox = await rescan.boundingBox();
    expect(beforeBox).not.toBeNull();
    expect(beforeBox!.height).toBeGreaterThanOrEqual(44);

    const event = testPage.getByRole("checkbox", {
      name: `Agent turn finished for ${APPRISE_PROVIDER_NAME}`,
    });
    await expect(event).not.toBeChecked();
    await event.tap();
    await expect(event).toBeChecked();

    startRescan();
    const detected = waitForHttp(testPage, "GET", /\/api\/v1\/notification-providers$/);
    await rescan.tap();
    await detected;
    await expect(testPage.getByText("Apprise detected.", { exact: true })).toBeVisible();
    await expect(testPage.getByRole("button", { name: "Add Apprise Provider" })).toBeVisible();
    await expect(
      testPage.getByRole("checkbox", { name: `Agent turn finished for ${APPRISE_PROVIDER_NAME}` }),
    ).toBeVisible();
    await expect(event).toBeChecked();
    await expect(testPage.getByRole("button", { name: "Save changes" })).toBeVisible();
    await prCapture.screenshot("apprise-rescan-mobile", {
      caption: "Mobile Apprise detection with a touch-sized rescan action",
    });

    const afterBox = await rescan.boundingBox();
    expect(afterBox).not.toBeNull();
    expect(afterBox!.height).toBeGreaterThanOrEqual(44);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);
  });

  test("keeps the rescan action touch-sized on a coarse tablet", async ({ tabletTestPage }) => {
    const { startRescan } = await routeAppriseRescans(tabletTestPage, [true]);
    const providersLoaded = waitForHttp(
      tabletTestPage,
      "GET",
      /\/api\/v1\/notification-providers$/,
    );
    await tabletTestPage.goto("/settings/preferences/notifications");
    await providersLoaded;

    await expect.poll(() => tabletTestPage.evaluate(() => window.innerWidth)).toBe(900);
    await expect
      .poll(() => tabletTestPage.evaluate(() => matchMedia("(pointer: coarse)").matches))
      .toBe(true);

    const rescan = tabletTestPage.getByTestId("apprise-rescan");
    await expect(rescan).toBeVisible();
    const box = await rescan.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    const event = tabletTestPage.getByRole("checkbox", {
      name: `Agent turn finished for ${APPRISE_PROVIDER_NAME}`,
    });
    await expect(event).not.toBeChecked();
    await event.tap();
    startRescan();
    const detected = waitForHttp(tabletTestPage, "GET", /\/api\/v1\/notification-providers$/);
    await rescan.tap();
    await detected;
    await expect(tabletTestPage.getByText("Apprise detected.", { exact: true })).toBeVisible();
    await expect(
      tabletTestPage.getByRole("button", { name: "Add Apprise Provider" }),
    ).toBeVisible();
    await expect(event).toBeChecked();
    await expect(tabletTestPage.getByRole("button", { name: "Save changes" })).toBeVisible();
    expect(
      await tabletTestPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);
  });
});
