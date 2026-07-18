import fs from "node:fs";
import { test, expect } from "../../fixtures/test-base";
import { seedManagedGoCache } from "../../helpers/storage-maintenance";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

test.describe("Mobile storage maintenance", () => {
  test("opens Storage from mobile navigation and analyzes without horizontal overflow", async ({
    testPage,
    backend,
  }) => {
    const cache = seedManagedGoCache(backend.tmpDir);
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileMenuButton.click();
    await testPage.getByRole("link", { name: "Settings" }).click();
    await testPage.getByTestId("settings-mobile-menu-button").click();
    const settingsMenu = testPage.getByTestId("settings-mobile-menu");
    await settingsMenu.getByRole("button", { name: "Expand System" }).click();
    await settingsMenu.getByRole("link", { name: "Storage" }).click();

    await expect(testPage.getByTestId("storage-settings-page")).toBeVisible();
    await testPage
      .getByRole("button", { name: "More information about Scheduled maintenance" })
      .click();
    await expect(testPage.getByRole("tooltip")).toContainText(
      "Turning it off does not disable Analyze or Run now",
    );
    await testPage.keyboard.press("Escape");
    await testPage.getByTestId("storage-scheduling-enabled").click();
    await testPage.getByTestId("storage-idle-period").fill("12");
    await expect(testPage.getByTestId("storage-policy-section-schedule")).toHaveAttribute(
      "data-settings-dirty",
      "true",
    );
    await expect(testPage.getByTestId("settings-floating-save")).toBeVisible();
    await testPage.getByRole("button", { name: "Save changes" }).click();
    await expect(testPage.getByText("Storage policy saved")).toBeVisible();
    await testPage.getByTestId("storage-analyze").click();
    await expect(testPage.getByTestId("storage-analyze")).toHaveAttribute(
      "data-job-state",
      "succeeded",
    );
    await expect(testPage.getByTestId("storage-analyze")).toHaveText("Analysis complete");
    await testPage.getByTestId("storage-resource-workspaces-trigger").click();
    await expect(testPage.getByTestId("storage-resource-workspaces")).toBeVisible();
    await testPage.getByTestId("storage-resource-go-cache-trigger").click();
    const explicitRequest = testPage.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        new URL(request.url()).pathname === "/api/v1/system/storage/run",
    );
    await testPage.getByTestId("storage-go-cache-clean").click();
    expect((await explicitRequest).postDataJSON()).toEqual({ resources: ["go_cache"] });
    await expect.poll(() => fs.existsSync(cache.artifact)).toBe(false);
    await testPage.getByRole("button", { name: "More information about Quarantine" }).click();
    await expect(testPage.getByRole("tooltip")).toContainText("recoverable holding area");
    await expect
      .poll(() =>
        testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      )
      .toBe(true);
  });
});
