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
    await testPage.getByTestId("storage-analyze").click();
    await expect(testPage.getByTestId("storage-analysis-job")).toHaveAttribute(
      "data-state",
      "succeeded",
    );
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
    await expect
      .poll(() =>
        testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      )
      .toBe(true);
  });
});
