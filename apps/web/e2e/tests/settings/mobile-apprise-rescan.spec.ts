import { test, expect } from "../../fixtures/test-base";
import { APPRISE_PROVIDER_NAME, routeAppriseRescans } from "./apprise-rescan-helpers";

test.describe("Mobile Apprise rescan", () => {
  test("detects Apprise with a touch-sized action and no horizontal overflow", async ({
    testPage,
    prCapture,
  }) => {
    const { startRescan } = await routeAppriseRescans(testPage, [true]);
    await testPage.goto("/settings/preferences/notifications");

    const rescan = testPage.getByTestId("apprise-rescan");
    await expect(rescan).toBeVisible();
    const beforeBox = await rescan.boundingBox();
    expect(beforeBox).not.toBeNull();
    expect(beforeBox!.height).toBeGreaterThanOrEqual(44);

    startRescan();
    await rescan.tap();
    await expect(testPage.getByText("Apprise detected.", { exact: true })).toBeVisible();
    await expect(testPage.getByRole("button", { name: "Add Apprise Provider" })).toBeVisible();
    await expect(
      testPage.getByRole("checkbox", { name: `Agent turn finished for ${APPRISE_PROVIDER_NAME}` }),
    ).toBeVisible();
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
});
